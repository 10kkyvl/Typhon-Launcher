//go:build !windows

package selfupdate

func RunWorker(specPath string) error {
	return ErrApplyUnsupported
}

func startUpdateWorker(exePath, specPath string) error {
	return ErrApplyUnsupported
}
