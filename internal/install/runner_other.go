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

// workerProcessAlive: воркер существует только на Windows, поэтому здесь его
// никогда нет — ложь без ошибки, а не отказ, потому что вызывающие относятся
// к "не воркер" и "воркер мёртв" одинаково.
func workerProcessAlive(int) (bool, error) {
	return false, nil
}
