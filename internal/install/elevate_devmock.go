//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var (
	errDevmockNoWorkerSpec = errors.New("devmock: аргумент --install-worker не найден")
	errDevmockNoWorkerID   = errors.New("devmock: у спецификации воркера нет id")
)

type devmockProc struct {
	cmd *exec.Cmd
}

func (p *devmockProc) wait() (int, error) {
	err := p.cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("ожидание воркера: %w", err)
	}
	return 0, nil
}

// Unix child processes own their descriptors and Wait reaps them; unlike the
// Windows process handle there is nothing left for the parent to release.
func (*devmockProc) close() {}

func workerStartError(path string, err error) error {
	return fmt.Errorf("запуск воркера установки %s: %w", path, err)
}

func startElevated(spec runSpec) (workerHandle, error) {
	logPath, err := devmockWorkerLogPath(spec)
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open worker log %s: %w", logPath, err)
	}
	//nolint:gosec // G204: spec.Path — путь к самому лаунчеру из os.Executable() (runElevated), аргументы — фиксированный флаг и путь spec-файла (инвариант 32)
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Setsid: the worker must survive a launcher restart (ServiceStartup resumes
	// it), so it cannot share the parent's session and die with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		if closeErr := logFile.Close(); closeErr != nil {
			slog.Warn("close worker log", "path", logPath, "error", closeErr)
		}
		return nil, fmt.Errorf("start %s: %w", spec.Path, err)
	}
	if err := logFile.Close(); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			slog.Warn("kill worker after log close failure", "path", spec.Path, "error", killErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			slog.Warn("reap worker after log close failure", "path", spec.Path, "error", waitErr)
		}
		return nil, fmt.Errorf("close worker log %s: %w", logPath, err)
	}
	return &devmockProc{cmd: cmd}, nil
}

func devmockWorkerLogPath(spec runSpec) (string, error) {
	if spec.StatePath != "" && spec.ID != "" {
		return devmockWorkerLog(filepath.Dir(spec.StatePath), spec.ID), nil
	}
	specFile, err := workerSpecArg(spec.Args)
	if err != nil {
		return "", err
	}
	ws, err := readWorkerSpec(specFile)
	if err != nil {
		return "", err
	}
	if ws.ID == "" {
		return "", fmt.Errorf("%w: %s", errDevmockNoWorkerID, specFile)
	}
	return devmockWorkerLog(filepath.Dir(specFile), ws.ID), nil
}

func devmockWorkerLog(dir, id string) string {
	return filepath.Join(dir, "worker-"+id+".log")
}

func workerSpecArg(args []string) (string, error) {
	for i, arg := range args {
		if arg == installWorkerFlag && i+1 < len(args) {
			return args[i+1], nil
		}
	}
	return "", errDevmockNoWorkerSpec
}

func attemptDiscovery(_ context.Context, in discoverySpec) (discoveryOutcome, error) {
	if !shouldDiscoverComponents(in) {
		return discoveryOutcome{}, nil
	}
	return discoveryOutcome{reason: "devmock: component discovery is not mocked"}, nil
}
