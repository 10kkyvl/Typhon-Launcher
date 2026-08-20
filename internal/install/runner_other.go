//go:build !windows

package install

import "context"

type unsupportedRunner struct{}

func newRunner() runner { return unsupportedRunner{} }

func (unsupportedRunner) run(context.Context, runSpec) (int, error) {
	return 0, errWindowsOnly
}
