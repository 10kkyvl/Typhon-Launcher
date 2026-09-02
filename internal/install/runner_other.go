//go:build !windows && !devmock

package install

import "context"

type unsupportedRunner struct{}

func newRunner(func() string) runner { return unsupportedRunner{} }

func (unsupportedRunner) run(context.Context, runSpec) (int, error) {
	return 0, errWindowsOnly
}
