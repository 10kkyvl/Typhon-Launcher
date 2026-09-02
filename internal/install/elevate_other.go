//go:build !windows && !devmock

package install

import "fmt"

func startElevated(runSpec) (workerHandle, error) {
	return nil, errWindowsOnly
}

func workerStartError(path string, err error) error {
	return fmt.Errorf("запуск воркера установки %s: %w", path, err)
}
