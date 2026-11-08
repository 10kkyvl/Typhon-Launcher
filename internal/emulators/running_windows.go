//go:build windows

package emulators

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/windows"

	"typhon/internal/platform"
	"typhon/internal/procs"
)

// On Windows RPCS3 keeps its data beside the program, so only a copy of the
// same program stands in the way of a new one.
var systemCopies = copyFinder{list: windowsCopies, ask: askToClose, force: terminate}

func windowsCopies(ctx context.Context, program string) ([]int, error) {
	list, _, err := procs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("таблица процессов: %w", err)
	}
	var out []int
	for _, p := range list {
		if !p.PathUnknown && platform.SamePath(p.Path, program) {
			out = append(out, int(p.PID))
		}
	}
	return out, nil
}

// askToClose sends the program's windows a close request, as the close button
// would: taskkill without /F does exactly that.
func askToClose(pid int) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("системная папка: %w", err)
	}
	//nolint:gosec // G204: taskkill by its absolute system path, the argument is a pid
	cmd := exec.Command(filepath.Join(dir, "taskkill.exe"), "/PID", strconv.Itoa(pid))
	if out, err := cmd.CombinedOutput(); err != nil {
		var exit *exec.ExitError
		// 128 is "no such process": it quit in the meantime.
		if errors.As(err, &exit) && exit.ExitCode() == 128 {
			return nil
		}
		return fmt.Errorf("taskkill: %w: %s", err, out)
	}
	return nil
}

func terminate(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("pid %d", pid)
	}
	//nolint:gosec // G115: a Windows pid fits in uint32, non-positive ones are refused above
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("открыть процесс: %w", err)
	}
	terminateErr := windows.TerminateProcess(handle, 1)
	closeErr := windows.CloseHandle(handle)
	return errors.Join(terminateErr, closeErr)
}
