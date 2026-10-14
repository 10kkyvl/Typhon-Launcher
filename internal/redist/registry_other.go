//go:build !windows

package redist

import "context"

func InstalledVC(context.Context) ([]Runtime, error) {
	return nil, ErrUnsupported
}
