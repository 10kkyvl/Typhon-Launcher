package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const installWorkerFlag = "--install-worker"

var (
	errWorkerStatePath   = errors.New("путь состояния установки не задан")
	errWorkerNotFinished = errors.New("повышенный воркер установки не подтвердил завершение")

	// Подменяются в тестах, чтобы не поднимать настоящий UAC-запрос и не ждать
	// боевые тайминги.
	startElevatedWorker = startElevated
	workerPollInterval  = 250 * time.Millisecond
	workerCancelWait    = 30 * time.Second
)

type workerHandle interface {
	wait() (int, error)
	close()
}

type elevatedResult struct {
	code int
	err  error
}

// Установщик сам по себе лаунчеру больше не принадлежит: вместо того чтобы
// поднимать его напрямую через ShellExecuteEx и держать в подвешенном
// состоянии до смерти процесса, лаунчер один раз поднимает собственный
// воркер (RunWorker в worker_run.go) с правами администратора и дальше
// общается с ним только через файлы spec/state/cancel. Воркер сам владеет
// установщиком, держит его в job-объекте и умеет прерывать разведку
// компонентов — то, что раньше было недостижимо для процесса, которым
// лаунчер не управляет.
func runElevated(ctx context.Context, spec runSpec) (int, error) {
	slog.Info("installer requires elevation", "path", spec.Path, "background", spec.Background)
	if spec.StatePath == "" {
		return 0, errWorkerStatePath
	}
	dir := filepath.Dir(spec.StatePath)
	specFile := workerSpecFilePath(dir, spec.ID)

	if err := clearWorkerCancel(spec.CancelPath); err != nil {
		return 0, fmt.Errorf("подготовка воркера установки: %w", err)
	}
	ws := workerSpec{
		ID:            spec.ID,
		InstallerPath: spec.InstallerPath,
		Engine:        spec.Engine,
		Destination:   spec.Destination,
		WorkingDir:    spec.Dir,
		LogPath:       spec.LogPath,
		InfPath:       spec.InfPath,
		StatePath:     spec.StatePath,
		CancelPath:    spec.CancelPath,
		Options:       spec.Options,
		Background:    spec.Background,
		Hidden:        true,
	}
	if err := writeWorkerSpec(specFile, ws); err != nil {
		return 0, fmt.Errorf("подготовка воркера установки: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("путь к лаунчеру: %w", err)
	}
	proc, err := startElevatedWorker(runSpec{Path: exe, Args: []string{installWorkerFlag, specFile}, Hidden: true})
	if err != nil {
		return 0, workerStartError(spec.Path, err)
	}
	defer proc.close()

	exited := make(chan elevatedResult, 1)
	go func() {
		code, waitErr := proc.wait()
		exited <- elevatedResult{code: code, err: waitErr}
	}()

	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()

	cancelRequested := false
	var cancelDeadline <-chan time.Time
	for {
		select {
		case res := <-exited:
			if res.err != nil {
				return 0, res.err
			}
			return readFinalWorkerState(spec.StatePath)
		case <-ctx.Done():
			if !cancelRequested {
				cancelRequested = true
				if err := writeWorkerCancel(spec.CancelPath); err != nil {
					slog.Warn("request installer worker cancellation", "path", spec.Path, "error", err)
				}
				cancelDeadline = time.After(workerCancelWait)
			}
		case <-cancelDeadline:
			return 0, fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, ctx.Err())
		case <-ticker.C:
			state, found, stateErr := readWorkerState(spec.StatePath)
			if stateErr != nil {
				return 0, fmt.Errorf("состояние установки: %w", stateErr)
			}
			if found && state.Done {
				return finishElevatedState(state)
			}
		}
	}
}

func readFinalWorkerState(statePath string) (int, error) {
	state, found, err := readWorkerState(statePath)
	if err != nil {
		return 0, fmt.Errorf("состояние установки: %w", err)
	}
	if !found || !state.Done {
		return 0, errWorkerNotFinished
	}
	return finishElevatedState(state)
}

// finishElevatedState оборачивает отмену через %w вокруг context.Canceled:
// Service.fail (service.go) решает, что делать с ошибкой, через
// errors.Is(cause, context.Canceled), а errors.New(state.Error) создаёт
// значение, не сравнимое ни с чем (инвариант 24 — отмена и провал
// установщика не одна и та же причина, и это должно быть видно вызывающему,
// а не только человеку, читающему текст).
func finishElevatedState(state workerState) (int, error) {
	switch {
	case state.Cancelled:
		return state.Code, fmt.Errorf("установка отменена: %w", context.Canceled)
	case state.Error != "":
		return state.Code, errors.New(state.Error)
	default:
		return state.Code, nil
	}
}
