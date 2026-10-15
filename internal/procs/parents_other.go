//go:build !windows

package procs

import (
	"context"
	"errors"
)

var errUnsupportedParents = errors.New("procs: parent process lookup is only supported on Windows")

func Parents(_ context.Context) (map[uint32]uint32, error) {
	return nil, errUnsupportedParents
}

func ParentsNow() (map[uint32]uint32, error) {
	return nil, errUnsupportedParents
}
