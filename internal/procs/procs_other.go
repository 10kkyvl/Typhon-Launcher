//go:build !windows && !devmock

package procs

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("procs: process enumeration is only supported on Windows")

func Supported() bool { return false }

func List(_ context.Context) ([]Process, error) {
	return nil, errUnsupported
}
