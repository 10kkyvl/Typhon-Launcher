//go:build windows

package install

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	errElevationDeclined = uierr.New("install.elevation_declined", "нужны права администратора: запрос Windows отклонён. Повторите действие и подтвердите запрос")
	errNoElevatedProcess = uierr.New("install.no_elevated_process", "процесс установщика с правами администратора не запустился")
)

// CreateProcess никогда не поднимает UAC: для установщика с requireAdministrator
// в манифесте он возвращает ERROR_ELEVATION_REQUIRED, и запросить права можно
// только через ShellExecuteEx с глаголом runas.
func needsElevation(err error) bool {
	return errors.Is(err, windows.ERROR_ELEVATION_REQUIRED)
}

// Отказ в окне UAC приходит как ERROR_CANCELLED и означает решение
// пользователя, а не сбой установки.
func elevationError(path string, err error) error {
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return errElevationDeclined
	}
	return fmt.Errorf("запуск установщика %s с правами администратора: %w", path, err)
}

type elevatedProc struct {
	mu     sync.Mutex
	handle windows.Handle
}

func (p *elevatedProc) terminate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return nil
	}
	return windows.TerminateProcess(p.handle, 1)
}

func (p *elevatedProc) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return
	}
	if err := windows.CloseHandle(p.handle); err != nil {
		slog.Warn("close elevated process handle", "error", err)
	}
	p.handle = 0
}

func (p *elevatedProc) wait() (int, error) {
	p.mu.Lock()
	handle := p.handle
	p.mu.Unlock()
	return awaitProcess(handle)
}

func awaitProcess(handle windows.Handle) (int, error) {
	event, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return 0, fmt.Errorf("ожидание установщика: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("ожидание установщика: код %d", event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return 0, fmt.Errorf("код возврата установщика: %w", err)
	}
	return int(code), nil
}

func workerStartError(path string, err error) error {
	return elevationError(path, err)
}

func elevationSupported() bool { return true }

func elevationParams(spec runSpec) string {
	if spec.Tail != "" {
		return spec.Tail
	}
	parts := make([]string, 0, len(spec.Args))
	for _, arg := range spec.Args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	return strings.Join(parts, " ")
}

func startElevated(spec runSpec) (workerHandle, error) {
	show := int32(windows.SW_SHOWNORMAL)
	if spec.Hidden {
		show = int32(windows.SW_HIDE)
	}
	handle, err := platform.ShellExecute("runas", spec.Path, elevationParams(spec), spec.Dir, show)
	if err != nil {
		return nil, err
	}
	if handle == 0 {
		return nil, errNoElevatedProcess
	}
	return &elevatedProc{handle: handle}, nil
}
