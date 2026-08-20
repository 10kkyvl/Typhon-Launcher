//go:build windows

package install

import (
	"context"
	"errors"
	"os/exec"
)

type processRunner struct{}

func newRunner() runner { return processRunner{} }

func (processRunner) run(ctx context.Context, spec runSpec) (int, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		if err != nil {
			return 0, err
		}
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
