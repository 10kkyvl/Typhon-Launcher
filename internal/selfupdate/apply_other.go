//go:build !windows

package selfupdate

import "context"

func Apply(ctx context.Context, installerPath string) error {
	return ErrApplyUnsupported
}
