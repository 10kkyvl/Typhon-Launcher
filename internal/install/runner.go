package install

import (
	"context"
	"errors"
)

var errWindowsOnly = errors.New("установка доступна только в Windows")

type runSpec struct {
	Path string
	Args []string
	Dir  string
}

type runner interface {
	run(ctx context.Context, spec runSpec) (int, error)
}
