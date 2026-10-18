package relocate

import (
	"errors"
	"syscall"
)

const errCrossDevice = syscall.Errno(17) // ERROR_NOT_SAME_DEVICE

func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
