//go:build windows

package redist

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"

	"typhon/internal/platform"
)

const waitSliceMs = 200

func msiexecPath() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("системный каталог: %w", err)
	}
	return filepath.Join(dir, "msiexec.exe"), nil
}

func runElevated(ctx context.Context, exe string, args []string, workDir string) (int, error) {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	handle, err := platform.ShellExecute("runas", exe, strings.Join(parts, " "), workDir, windows.SW_HIDE)
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, ErrElevationDeclined
		}
		return 0, fmt.Errorf("запуск установщика: %w", err)
	}
	if handle == 0 {
		return 0, errors.New("оболочка не вернула процесс установщика")
	}
	code, waitErr := waitHandle(ctx, handle)
	if closeErr := windows.CloseHandle(handle); closeErr != nil && waitErr == nil {
		waitErr = fmt.Errorf("закрыть хэндл установщика: %w", closeErr)
	}
	if waitErr != nil {
		return 0, waitErr
	}
	return code, nil
}

func waitHandle(ctx context.Context, handle windows.Handle) (int, error) {
	for {
		event, err := windows.WaitForSingleObject(handle, waitSliceMs)
		if err != nil {
			return 0, fmt.Errorf("ожидание установщика: %w", err)
		}
		switch event {
		case windows.WAIT_OBJECT_0:
			var code uint32
			if err := windows.GetExitCodeProcess(handle, &code); err != nil {
				return 0, fmt.Errorf("код выхода установщика: %w", err)
			}
			return int(code), nil
		case uint32(windows.WAIT_TIMEOUT):
			if ctxErr := ctx.Err(); ctxErr != nil {
				return 0, cancelRun(ctxErr, func() error { return windows.TerminateProcess(handle, 1) })
			}
		default:
			return 0, fmt.Errorf("ожидание установщика: код %d", event)
		}
	}
}
