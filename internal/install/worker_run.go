package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var workerCancelPollInterval = 250 * time.Millisecond

// RunWorker — точка входа отдельного процесса с правами администратора: сам
// лаунчер поднимает его один раз через startElevated и дальше общается с ним
// только через файлы spec/state/cancel, потому что процесс с высоким уровнем
// целостности лаунчеру не принадлежит и других каналов связи для него нет.
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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		watchWorkerCancel(ctx, spec.CancelPath, cancel)
	}()
	defer func() {
		cancel()
		wg.Wait()
	}()

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
		InstallerPath: spec.InstallerPath, Destination: spec.Destination, LogPath: spec.LogPath,
	}
	// Neither runner needs gamesPath here: the worker only ever runs the main
	// silent install with spec.Destination already resolved, never the
	// devmock placement path under the library root.
	return newRunner(func() string { return "" }).run(ctx, rs)
}

// applyDiscoveredComponents дописывает /COMPONENTS в уже готовый план
// основного прогона: spec.Args строился в silentSpec (flow.go) до того, как
// стал известен список компонентов, поэтому план приходится пересобирать
// через silentArgs заново, а не патчить готовую строку.
func applyDiscoveredComponents(spec runSpec, components []string) (runSpec, error) {
	if len(components) == 0 {
		return spec, nil
	}
	plan, err := silentArgs(spec.Engine, spec.InstallerPath, spec.Destination, spec.LogPath, spec.Options)
	if err != nil {
		return runSpec{}, err
	}
	plan = planWithComponents(plan, components)
	spec.Args = plan.Args
	spec.CmdLine = plan.CmdLine
	spec.Tail = plan.Tail
	return spec, nil
}
