//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const backgroundSchedulingClass = 3

type processRunner struct{}

func newRunner(func() string) runner { return processRunner{} }

// run сначала пытается разведать компоненты Inno (если движок и опции того
// требуют) под своими текущими правами — так GOG-репаки, которым UAC не
// нужен, тоже получают /COMPONENTS, а не только установки, ушедшие в
// runElevated. Требование повышения на разведке значит, что и основной
// установщик его потребует: тогда всей установкой целиком занимается воркер.
func (processRunner) run(ctx context.Context, spec runSpec) (int, error) {
	outcome, err := attemptDiscovery(ctx, spec.discovery())
	if err != nil {
		return 0, err
	}
	if outcome.elevate {
		return runElevated(ctx, spec)
	}
	if outcome.reason != "" {
		slog.Warn("component discovery skipped", "path", spec.Path, "reason", outcome.reason)
	}

	execSpec, err := applyDiscoveredComponents(spec, outcome.components)
	if err != nil {
		return 0, err
	}

	//nolint:gosec // G204: путь и аргументы установщика проверены в Inspect/Start (инвариант 32); переменный путь — сама суть install-сервиса
	cmd := exec.Command(execSpec.Path, execSpec.Args...)
	cmd.Dir = execSpec.Dir
	cmd.SysProcAttr = startupAttr(execSpec)
	if err := cmd.Start(); err != nil {
		if needsElevation(err) {
			return runElevated(ctx, spec)
		}
		return 0, err
	}

	// Установщик распаковывает себя во временный каталог и работает уже оттуда:
	// без job-объекта отмена убила бы только загрузчик, а установка продолжилась бы.
	group, groupErr := groupProcess(cmd.Process.Pid, execSpec.Background)
	if groupErr != nil {
		slog.Warn("installer job object", "path", execSpec.Path, "error", groupErr)
	}
	defer closeGroup(group, execSpec.Path)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		if err != nil {
			return 0, err
		}
		return 0, nil
	case <-ctx.Done():
		terminateGroup(group, cmd, execSpec.Path)
		<-done
		return 0, ctx.Err()
	}
}

func startupAttr(spec runSpec) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	// NSIS разбирает хвост командной строки сам: /D=<путь> обязан идти последним
	// и без кавычек, поэтому строку собираем вручную вместо экранирования Args.
	if spec.CmdLine != "" {
		attr.CmdLine = spec.CmdLine
	}
	// Право забрать передний план установщик наследует от лаунчера, который его
	// запустил: скрытое стартовое окно лишает его возможности выдернуть фокус из
	// полноэкранной игры. Скрывать можно только то, что заведомо ничего не
	// спрашивает: собственный MsgBox из скрипта установщика ключами тишины не
	// подавляется, и у скрытого процесса такое окно легко потерять.
	if spec.Hidden {
		attr.HideWindow = true
	}
	// Пониженный класс приоритета наследуют и порождённые процессы, где и идёт
	// распаковка.
	if spec.Background {
		attr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	return attr
}

func groupProcess(pid int, background bool) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	if background {
		if err := limitJob(job); err != nil {
			closeGroup(job, "")
			return 0, err
		}
	}
	//nolint:gosec // G115: pid, выданный ядром, в uint32 помещается всегда
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		closeGroup(job, "")
		return 0, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer func() {
		if err := windows.CloseHandle(proc); err != nil {
			slog.Warn("close process handle", "pid", pid, "error", err)
		}
	}()
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		closeGroup(job, "")
		return 0, fmt.Errorf("assign process %d to job: %w", pid, err)
	}
	return job, nil
}

// limitJob ставит KILL_ON_JOB_CLOSE: смерть воркера (или любого другого
// владельца job-объекта) закрывает его хэндл, и без этого флага установщик
// остаётся сиротой, а не гаснет вместе с процессом, который им управлял.
// Хэндл job-объекта во всех вызывающих закрывается уже после подтверждённого
// выхода процесса (cmd.Wait() отработал), поэтому обычное завершение флаг не
// затрагивает.
// limitJob сначала пробует набор лимитов вместе с KILL_ON_JOB_CLOSE: смерть
// владельца job-объекта (в первую очередь — воркера) тогда гасит и
// установщик, вместо того чтобы оставлять его сиротой. На части систем
// SetInformationJobObject отклоняет именно этот флаг с ERROR_INVALID_PARAMETER
// (воспроизведено на этой машине — похоже на вмешательство защитного ПО,
// PRIORITY_CLASS/SCHEDULING_CLASS сами по себе проходят). Понижение
// приоритета важнее автопогребения сироты, поэтому при отказе комбинированного
// набора лимитов повторяем без KILL_ON_JOB_CLOSE, а не проваливаем запуск
// установщика целиком.
func limitJob(job windows.Handle) error {
	info := windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
		LimitFlags: windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS | windows.JOB_OBJECT_LIMIT_SCHEDULING_CLASS |
			windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		PriorityClass:   windows.BELOW_NORMAL_PRIORITY_CLASS,
		SchedulingClass: backgroundSchedulingClass,
	}
	if err := setJobBasicLimits(job, info); err != nil {
		slog.Warn("job object kill-on-close unavailable, falling back", "error", err)
		info.LimitFlags = windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS | windows.JOB_OBJECT_LIMIT_SCHEDULING_CLASS
		if err := setJobBasicLimits(job, info); err != nil {
			return fmt.Errorf("limit job object: %w", err)
		}
	}
	return nil
}

func setJobBasicLimits(job windows.Handle, info windows.JOBOBJECT_BASIC_LIMIT_INFORMATION) error {
	//nolint:gosec // G103: SetInformationJobObject принимает структуру только по указателю; инвариант 22 требует управлять деревом процессов установщика
	ptr := uintptr(unsafe.Pointer(&info))
	_, err := windows.SetInformationJobObject(job, windows.JobObjectBasicLimitInformation, ptr, uint32(unsafe.Sizeof(info)))
	return err
}

func terminateGroup(job windows.Handle, cmd *exec.Cmd, path string) {
	if job != 0 {
		if err := windows.TerminateJobObject(job, 1); err != nil {
			slog.Warn("terminate installer group", "path", path, "error", err)
		}
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		slog.Warn("kill installer", "path", path, "error", err)
	}
}

const stillActive = 259

// workerProcessAlive отвечает на вопрос "жив ли этот PID" для восстановления
// после перезапуска лаунчера (ServiceStartup) и для отказа Retry поверх
// живого воркера: os.FindProcess на Windows всегда успешен вне зависимости
// от того, жив ли процесс, поэтому единственный надёжный способ —
// OpenProcess + GetExitCodeProcess. PROCESS_QUERY_LIMITED_INFORMATION
// специально рассчитан на то, чтобы процесс без прав администратора мог
// опросить процесс с более высоким уровнем целостности (воркер элевирован),
// в отличие от PROCESS_QUERY_INFORMATION.
func workerProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	//nolint:gosec // G115: pid прочитан из собственного файла состояния воркера, в uint32 помещается всегда
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, fmt.Errorf("open worker process %d: %w", pid, err)
	}
	defer func() {
		if cerr := windows.CloseHandle(handle); cerr != nil {
			slog.Warn("close worker process handle", "pid", pid, "error", cerr)
		}
	}()
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false, fmt.Errorf("worker process %d exit code: %w", pid, err)
	}
	return code == stillActive, nil
}

func closeGroup(job windows.Handle, path string) {
	if job == 0 {
		return
	}
	if err := windows.CloseHandle(job); err != nil {
		slog.Warn("close job object", "path", path, "error", err)
	}
}
