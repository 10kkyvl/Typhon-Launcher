//go:build !windows && !devmock && !darwin

package selfupdate

import "context"

func Apply(ctx context.Context, installerPath, installDir, target string) error {
	return ErrApplyUnsupported
}

func ApplyPortable(ctx context.Context, zipPath, target string) error {
	return ErrApplyUnsupported
}
