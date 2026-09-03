//go:build !windows && !devmock

package selfupdate

func RunWorker(specPath string) error {
	return ErrApplyUnsupported
}

func startUpdateWorker(exePath, specPath string) error {
	return ErrApplyUnsupported
}

func workerProcessAlive(pid int) (bool, error) {
	return false, ErrApplyUnsupported
}

func relaunch(path string) error {
	return ErrApplyUnsupported
}
