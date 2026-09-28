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
	"unsafe"

	"golang.org/x/sys/windows"

	"typhon/internal/platform"
)

var errElevatedNoProcess = errors.New("процесс игры, запущенной с правами администратора, не получен от системы")

func newGameStarter() gameStarter { return elevatingStarter(createProcessStarter, startElevated) }

// Explorer запускает игру с окружением пользователя и без стандартных
// хэндлов. os/exec отдаёт ей окружение лаунчера (WEBVIEW2_*, а у сборки из
// терминала — всё окружение терминала) и stdin/stdout/stderr на NUL.
func createProcessStarter(_ context.Context, req launch) (gameProcess, error) {
	appName, err := windows.UTF16PtrFromString(req.executable)
	if err != nil {
		return nil, fmt.Errorf("путь к игре: %w", err)
	}
	cmdLine, err := windows.UTF16PtrFromString(gameCommandLine(req.executable, req.args))
	if err != nil {
		return nil, fmt.Errorf("командная строка игры: %w", err)
	}
	var workDir *uint16
	if req.workDir != "" {
		workDir, err = windows.UTF16PtrFromString(req.workDir)
		if err != nil {
			return nil, fmt.Errorf("рабочая папка игры: %w", err)
		}
	}

	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, windows.GetCurrentProcessToken(), false); err != nil {
		return nil, fmt.Errorf("окружение игры: %w", err)
	}
	defer func() {
		if destroyErr := windows.DestroyEnvironmentBlock(env); destroyErr != nil {
			slog.Error("destroy game environment block", "executable", req.executable, "error", destroyErr)
		}
	}()

	si := windows.StartupInfo{Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_SHOWNORMAL}
	//nolint:gosec // G115: размер собственной структуры в uint32 помещается
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_DEFAULT_ERROR_MODE)
	if err := windows.CreateProcess(appName, cmdLine, nil, nil, false, flags, env, workDir, &si, &pi); err != nil {
		return nil, fmt.Errorf("запуск процесса игры: %w", err)
	}
	if closeErr := windows.CloseHandle(pi.Thread); closeErr != nil {
		slog.Error("close game thread handle", "executable", req.executable, "error", closeErr)
	}
	return &handleProcess{handle: pi.Process, id: int(pi.ProcessId)}, nil
}

// Экранирование то же, что у os/exec: аргументы игр не должны поменяться.
func gameCommandLine(executable string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(executable))
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	return strings.Join(parts, " ")
}

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

type windowsExitError struct{ code uint32 }

func (e *windowsExitError) Error() string {
	return fmt.Sprintf("процесс игры завершился с кодом %d", e.code)
}

func (e *windowsExitError) ExitCode() int { return int(e.code) }

func (p *handleProcess) wait() error {
	p.mu.Lock()
	handle := p.handle
	p.mu.Unlock()
	event, waitErr := windows.WaitForSingleObject(handle, windows.INFINITE)
	var code uint32
	var codeErr error
	if waitErr == nil && event == windows.WAIT_OBJECT_0 {
		codeErr = windows.GetExitCodeProcess(handle, &code)
	}
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
	if codeErr != nil {
		return fmt.Errorf("код выхода процесса игры: %w", codeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("закрыть хэндл процесса игры: %w", closeErr)
	}
	if code != 0 {
		return &windowsExitError{code: code}
	}
	return nil
}
