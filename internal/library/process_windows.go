//go:build windows && !devmock

package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"typhon/internal/platform"
)

var errElevatedNoProcess = errors.New("процесс игры, запущенной с правами администратора, не получен от системы")

func newGameStarter() gameStarter { return elevatingStarter(execStarter, startElevated) }

// CreateProcess никогда не поднимает UAC: игре с requireAdministrator в
// манифесте или с флагом совместимости «от имени администратора» он отвечает
// ERROR_ELEVATION_REQUIRED, тогда как Explorer идёт через оболочку и получает
// запрос прав. Повторить это можно только через ShellExecuteEx с runas.
func elevatingStarter(start gameStarter, elevate func(req launch) (gameProcess, error)) gameStarter {
	return func(ctx context.Context, req launch) (gameProcess, error) {
		proc, err := start(ctx, req)
		if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
			return proc, err
		}
		slog.Info("game requires elevation", "executable", req.executable)
		return elevate(req)
	}
}

func startElevated(req launch) (gameProcess, error) {
	parts := make([]string, 0, len(req.args))
	for _, arg := range req.args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	handle, err := platform.ShellExecute("runas", req.executable, strings.Join(parts, " "), req.workDir, windows.SW_SHOWNORMAL)
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return nil, errElevationDeclined
		}
		return nil, fmt.Errorf("запуск с правами администратора: %w", err)
	}
	if handle == 0 {
		return nil, errElevatedNoProcess
	}
	pid, err := windows.GetProcessId(handle)
	if err != nil {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			return nil, fmt.Errorf("pid игры: %w (закрыть хэндл: %w)", err, closeErr)
		}
		return nil, fmt.Errorf("pid игры: %w", err)
	}
	return &handleProcess{handle: handle, id: int(pid)}, nil
}

type handleProcess struct {
	mu     sync.Mutex
	handle windows.Handle
	id     int
}

func (p *handleProcess) pid() int { return p.id }

func (p *handleProcess) kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return errSessionProcessGone
	}
	return windows.TerminateProcess(p.handle, 1)
}

func (p *handleProcess) wait() error {
	p.mu.Lock()
	handle := p.handle
	p.mu.Unlock()
	event, waitErr := windows.WaitForSingleObject(handle, windows.INFINITE)
	p.mu.Lock()
	defer p.mu.Unlock()
	closeErr := windows.CloseHandle(p.handle)
	p.handle = 0
	if waitErr != nil {
		return fmt.Errorf("ожидание процесса игры: %w", waitErr)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("ожидание процесса игры: код %d", event)
	}
	if closeErr != nil {
		return fmt.Errorf("закрыть хэндл процесса игры: %w", closeErr)
	}
	return nil
}
