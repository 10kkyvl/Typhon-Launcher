//go:build !windows

package lan

import (
	"syscall"
	"testing"
)

func makeFifo(t *testing.T, path string) error {
	t.Helper()
	return syscall.Mkfifo(path, 0o644)
}
