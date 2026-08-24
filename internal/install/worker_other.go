//go:build !windows

package install

func RunWorker(string) error {
	return errWindowsOnly
}
