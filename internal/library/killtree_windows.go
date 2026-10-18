//go:build windows

package library

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"

	"typhon/internal/procs"
)

func killProcessTree(root int, killRoot func() error) error {
	if root <= 0 {
		return errNoRootPID
	}
	//nolint:gosec // G115: PID из ОС укладывается в uint32 на Windows, отрицательные отсечены выше
	return killTree(uint32(root), procs.ParentsNow, terminatePID, killRoot)
}

func terminatePID(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		// Процесс успел выйти между снимком и убийством: цели уже нет.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("открыть процесс: %w", err)
	}
	terminateErr := windows.TerminateProcess(handle, 1)
	closeErr := windows.CloseHandle(handle)
	return errors.Join(terminateErr, closeErr)
}
