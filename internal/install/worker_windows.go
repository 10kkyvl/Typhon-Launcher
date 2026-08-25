//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var (
	discoveryPollInterval    = 200 * time.Millisecond
	discoveryTimeout         = 60 * time.Second
	workerCancelPollInterval = 250 * time.Millisecond
)

// RunWorker — точка входа отдельного процесса с правами администратора: сам
// лаунчер поднимает его один раз через startElevated (runner_windows.go) и
// дальше общается с ним только через файлы spec/state/cancel, потому что
// процесс с высоким уровнем целостности лаунчеру не принадлежит и других
// каналов связи для него нет.
//
//nolint:forbidigo // RunWorker — точка входа отдельного процесса, эквивалент main для воркера: вызывающего ctx нет (инвариант 20 разрешает Background только в main)
func RunWorker(specPath string) error {
	spec, specErr := readWorkerSpec(specPath)
	if specErr != nil {
		if spec.StatePath != "" {
			if err := writeWorkerState(spec.StatePath, workerState{Done: true, Error: specErr.Error()}); err != nil {
				return fmt.Errorf("%w (не удалось записать состояние воркера: %w)", specErr, err)
			}
		}
		return specErr
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		watchWorkerCancel(ctx, spec.CancelPath, cancel)
	}()
	defer wg.Wait()

	state := workerState{PID: os.Getpid(), Phase: string(workerPhaseDiscovering)}
	if err := writeWorkerState(spec.StatePath, state); err != nil {
		return err
	}

	components, reason, err := discoverComponents(ctx, spec.discovery())
	if err != nil {
		return finishWorkerState(spec.StatePath, state, err)
	}
	state.DiscoveryFailure = reason
	state.Components = components
	state.Phase = string(workerPhaseInstalling)
	if err := writeWorkerState(spec.StatePath, state); err != nil {
		return err
	}

	code, runErr := runMainInstall(ctx, spec, components)
	state.Code = code
	state.Done = true
	if runErr != nil {
		state.Error = runErr.Error()
		state.Cancelled = errors.Is(runErr, context.Canceled)
	}
	if err := writeWorkerState(spec.StatePath, state); err != nil {
		if runErr != nil {
			return fmt.Errorf("%w (не удалось записать состояние воркера: %w)", runErr, err)
		}
		return err
	}
	return runErr
}

// finishWorkerState записывает финальное состояние с причиной сбоя до того,
// как разведка компонентов провалилась настолько, что дальше продолжать
// нельзя (в отличие от неудачной разведки, которая не ошибка, а причина в
// DiscoveryFailure). Отмена помечается отдельным флагом, а не только текстом:
// Service.fail (service.go) различает отмену и провал через errors.Is, а
// errors.New(state.Error) для этого непригоден — новая ошибка не сравнивается
// с context.Canceled (инвариант 24).
func finishWorkerState(statePath string, state workerState, cause error) error {
	state.Done = true
	state.Error = cause.Error()
	state.Cancelled = errors.Is(cause, context.Canceled)
	if err := writeWorkerState(statePath, state); err != nil {
		return fmt.Errorf("%w (не удалось записать состояние воркера: %w)", cause, err)
	}
	return cause
}

func watchWorkerCancel(ctx context.Context, path string, cancel context.CancelFunc) {
	if path == "" {
		return
	}
	ticker := time.NewTicker(workerCancelPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if workerCancelRequested(path) {
				cancel()
				return
			}
		}
	}
}

// discoverComponents — обёртка attemptDiscovery для воркера: воркер уже
// поднят с правами администратора, поэтому запрос повышения на этом пути
// невозможен в норме, а если всё же случился — это не повод останавливать
// установку, только причина неудачной разведки.
func discoverComponents(ctx context.Context, in discoverySpec) ([]string, string, error) {
	outcome, err := attemptDiscovery(ctx, in)
	if err != nil {
		return nil, "", err
	}
	if outcome.elevate {
		return nil, "запуск установщика для разведки потребовал повышения прав", nil
	}
	return outcome.components, outcome.reason, nil
}

type discoveryOutcome struct {
	components []string
	reason     string
	// elevate означает, что даже попытку разведки нельзя стартовать без
	// прав администратора: вызывающий на неэлевированном пути обязан
	// целиком передать установку воркеру через runElevated, а не только
	// разведку.
	elevate bool
}

func shouldDiscoverComponents(in discoverySpec) bool {
	return in.Engine == EngineInno && (in.Options.SkipExtras || in.Options.SkipShortcuts)
}

// attemptDiscovery — единственное место, которое умеет проводить разведку
// компонентов Inno: пользуются им и воркер (уже с правами администратора), и
// обычный неэлевированный путь запуска (processRunner.run в
// runner_windows.go), которому без разведки установщик без UAC ставил бы
// DirectX/vcredist ровно как до появления воркера (инвариант 28 — один
// источник правды на понятие).
func attemptDiscovery(ctx context.Context, in discoverySpec) (discoveryOutcome, error) {
	if !shouldDiscoverComponents(in) {
		return discoveryOutcome{}, nil
	}

	plan, ok, err := discoverPlan(in.Engine, in.InstallerPath, in.Destination, in.InfPath)
	if err != nil {
		return discoveryOutcome{}, fmt.Errorf("построение плана разведки: %w", err)
	}
	if !ok {
		return discoveryOutcome{reason: "движок не поддерживает разведку компонентов"}, nil
	}
	if err := os.Remove(in.InfPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return discoveryOutcome{}, fmt.Errorf("удаление старого файла разведки: %w", err)
	}

	rs := runSpec{Path: in.InstallerPath, Args: plan.Args, CmdLine: plan.CmdLine, Dir: in.WorkingDir, Hidden: true, Background: true}
	cmd, group, startErr := startDiscoveryRun(rs)
	if startErr != nil {
		if needsElevation(startErr) {
			return discoveryOutcome{elevate: true}, nil
		}
		return discoveryOutcome{reason: fmt.Sprintf("запуск установщика для разведки: %v", startErr)}, nil
	}

	reason, err := awaitDiscoveryIni(ctx, cmd, group, in.InfPath, in.InstallerPath)
	if err != nil {
		return discoveryOutcome{}, err
	}
	if reason != "" {
		return discoveryOutcome{reason: reason}, nil
	}

	components, readReason, err := readDiscoveredComponents(in.InfPath, in.Options)
	if err != nil {
		return discoveryOutcome{}, err
	}
	return discoveryOutcome{components: components, reason: readReason}, nil
}

// startDiscoveryRun запускает установщик с /SAVEINF и заводит для него
// отдельный job-объект: разведку нужно оборвать до того, как установщик
// закончит настоящую установку, а без job-объекта оборвать дерево процессов
// Inno нечем.
func startDiscoveryRun(rs runSpec) (*exec.Cmd, windows.Handle, error) {
	//nolint:gosec // G204: воркер запускает установщик, путь которого уже проверен в Inspect/Start (инвариант 32); переменный путь — сама суть install-сервиса
	cmd := exec.Command(rs.Path, rs.Args...)
	cmd.Dir = rs.Dir
	cmd.SysProcAttr = startupAttr(rs)
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}

	group, groupErr := groupProcess(cmd.Process.Pid, true)
	if groupErr != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			slog.Warn("kill discovery installer", "path", rs.Path, "error", killErr)
		}
		return nil, 0, fmt.Errorf("job object разведки: %w", groupErr)
	}
	return cmd, group, nil
}

// awaitDiscoveryIni обрывает уже запущенный разведывательный прогон сразу,
// как только появляется файл разведки, или по истечении потолка ожидания:
// доводить этот прогон до конца не нужно, разведка — не установка.
func awaitDiscoveryIni(ctx context.Context, cmd *exec.Cmd, group windows.Handle, infPath, installerPath string) (string, error) {
	defer closeGroup(group, installerPath)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(discoveryPollInterval)
	defer ticker.Stop()
	deadline := time.After(discoveryTimeout)

	reason := ""
	exited := false
wait:
	for {
		if _, err := os.Stat(infPath); err == nil {
			break wait
		}
		select {
		case <-done:
			exited = true
			reason = "установщик завершился раньше, чем появился файл разведки"
			break wait
		case <-ctx.Done():
			terminateGroup(group, cmd, installerPath)
			<-done
			return "", ctx.Err()
		case <-deadline:
			reason = "файл разведки не появился за отведённое время"
			break wait
		case <-ticker.C:
		}
	}

	terminateGroup(group, cmd, installerPath)
	// done уже осушён веткой case <-done: выше — второй приём на том же
	// буферизованном канале никогда не разблокируется, потому что cmd.Wait()
	// отправляет в него ровно один раз.
	if !exited {
		<-done
	}
	return reason, nil
}

func readDiscoveredComponents(infPath string, opts installOptions) ([]string, string, error) {
	data, err := os.ReadFile(infPath)
	if err != nil {
		return nil, fmt.Sprintf("чтение файла разведки: %v", err), nil
	}
	list, ok := infComponents(data)
	if !ok {
		return nil, "секция Components не найдена в файле разведки", nil
	}
	filtered, changed := filterComponents(list, opts)
	if !changed {
		return nil, "", nil
	}
	return filtered, "", nil
}

func runMainInstall(ctx context.Context, spec workerSpec, components []string) (int, error) {
	plan, err := silentArgs(spec.Engine, spec.InstallerPath, spec.Destination, spec.LogPath, spec.Options)
	if err != nil {
		return 0, err
	}
	if len(components) > 0 {
		plan = planWithComponents(plan, components)
	}
	path := spec.InstallerPath
	if spec.Engine == EngineMsi {
		msiexec, err := systemExecutable("msiexec.exe")
		if err != nil {
			return 0, err
		}
		path = msiexec
	}
	rs := runSpec{
		Path: path, Args: plan.Args, Dir: spec.WorkingDir, CmdLine: plan.CmdLine, Tail: plan.Tail,
		Background: spec.Background, Hidden: spec.Hidden,
	}
	return newRunner().run(ctx, rs)
}
