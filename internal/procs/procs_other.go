//go:build !windows && !devmock && !darwin

package procs

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("procs: process enumeration is only supported on Windows")

func Supported() bool { return false }

func List(_ context.Context) ([]Process, bool, error) {
	return nil, false, errUnsupported
}
