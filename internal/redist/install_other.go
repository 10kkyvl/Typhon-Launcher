//go:build !windows

package redist

import "context"

func runElevated(context.Context, string, []string, string) (int, error) {
	return 0, ErrUnsupported
}

func msiexecPath() (string, error) {
	return "", ErrUnsupported
}
