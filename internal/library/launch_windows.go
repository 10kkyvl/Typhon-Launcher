package library

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"

	"typhon/internal/platform"

	"golang.org/x/sys/windows"
)

// CreateProcess никогда не поднимает UAC: игре с requireAdministrator в
// манифесте или с флагом совместимости «от имени администратора» он отвечает
// ERROR_ELEVATION_REQUIRED, тогда как Explorer идёт через оболочку и получает
// запрос прав. Повторить это можно только через ShellExecuteEx с runas.
func needsElevation(err error) bool {
	return errors.Is(err, windows.ERROR_ELEVATION_REQUIRED)
}

func startElevated(exe string, args []string, dir string) (launched, error) {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	handle, err := platform.ShellExecute("runas", exe, strings.Join(parts, " "), dir, windows.SW_SHOWNORMAL)
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return launched{}, errElevationDeclined
		}
		return launched{}, fmt.Errorf("запуск с правами администратора: %w", err)
	}
	if handle == 0 {
		return launched{}, errElevatedNoProcess
	}
	pid, err := windows.GetProcessId(handle)
	if err != nil {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			return launched{}, fmt.Errorf("pid игры: %w (закрыть хэндл: %w)", err, closeErr)
		}
		return launched{}, fmt.Errorf("pid игры: %w", err)
	}
	proc := &handleProcess{handle: handle}
	return launched{process: proc, pid: pid, wait: proc.wait}, nil
}

type handleProcess struct {
	mu     sync.Mutex
	handle windows.Handle
}

func (p *handleProcess) Kill() error {
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
