//go:build devmock && !windows

package install

import (
	"errors"
	"fmt"
	"syscall"
)

// kill(pid, 0) delivers nothing and only reports whether the pid exists:
// EPERM means it exists but belongs to another user, ESRCH that it is gone.
func workerProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, fmt.Errorf("check worker process %d: %w", pid, err)
	}
}
