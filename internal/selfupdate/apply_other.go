//go:build !windows

package selfupdate

import "context"

func Apply(ctx context.Context, installerPath, installDir string) error {
	return ErrApplyUnsupported
}
