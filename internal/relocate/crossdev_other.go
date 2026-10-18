//go:build !windows

package relocate

import (
	"errors"
	"syscall"
)

const errCrossDevice = syscall.EXDEV

func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
