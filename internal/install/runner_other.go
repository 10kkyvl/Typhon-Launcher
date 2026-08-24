//go:build !windows

package install

import (
	"context"
	"errors"
)

var errWindowsOnly = errors.New("установка доступна только в Windows")

type unsupportedRunner struct{}

func newRunner() runner { return unsupportedRunner{} }

func (unsupportedRunner) run(context.Context, runSpec) (int, error) {
	return 0, errWindowsOnly
}
