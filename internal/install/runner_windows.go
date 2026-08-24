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

func newRunner() runner { return processRunner{} }

func (processRunner) run(ctx context.Context, spec runSpec) (int, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.SysProcAttr = startupAttr(spec)
	if err := cmd.Start(); err != nil {
		if needsElevation(err) {
			return runElevated(ctx, spec)
		}
		return 0, err
	}

	// Установщик распаковывает себя во временный каталог и работает уже оттуда:
	// без job-объекта отмена убила бы только загрузчик, а установка продолжилась бы.
	group, groupErr := groupProcess(cmd.Process.Pid, spec.Background)
	if groupErr != nil {
		slog.Warn("installer job object", "path", spec.Path, "error", groupErr)
	}
	defer closeGroup(group, spec.Path)

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
		terminateGroup(group, cmd, spec.Path)
		<-done
		return 0, ctx.Err()
	}
}

type elevatedResult struct {
	code int
	err  error
}

// Процесс с высоким уровнем целостности лаунчер не назначает в job-объект и не
// может ни понизить ему приоритет, ни завершить его: TerminateProcess вернёт
// отказ в доступе. Поэтому отмена по ctx для такой установки означает только,
// что лаунчер перестал её ждать.
func runElevated(ctx context.Context, spec runSpec) (int, error) {
	slog.Info("installer requires elevation", "path", spec.Path, "background", spec.Background)
	proc, err := startElevated(spec)
	if err != nil {
		return 0, elevationError(spec.Path, err)
	}

	done := make(chan elevatedResult, 1)
	go func() {
		defer proc.close()
		code, waitErr := awaitProcess(proc.handle)
		done <- elevatedResult{code: code, err: waitErr}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return 0, res.err
		}
		return res.code, nil
	case <-ctx.Done():
		if err := proc.terminate(); err != nil {
			slog.Warn("elevated installer keeps running", "path", spec.Path, "error", err)
		}
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

func limitJob(job windows.Handle) error {
	info := windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
		LimitFlags:      windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS | windows.JOB_OBJECT_LIMIT_SCHEDULING_CLASS,
		PriorityClass:   windows.BELOW_NORMAL_PRIORITY_CLASS,
		SchedulingClass: backgroundSchedulingClass,
	}
	//nolint:gosec // G103: SetInformationJobObject принимает структуру только по указателю; инвариант 22 требует управлять деревом процессов установщика
	ptr := uintptr(unsafe.Pointer(&info))
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectBasicLimitInformation, ptr, uint32(unsafe.Sizeof(info))); err != nil {
		return fmt.Errorf("limit job object: %w", err)
	}
	return nil
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

func closeGroup(job windows.Handle, path string) {
	if job == 0 {
		return
	}
	if err := windows.CloseHandle(job); err != nil {
		slog.Warn("close job object", "path", path, "error", err)
	}
}
