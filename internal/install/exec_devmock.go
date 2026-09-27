//go:build devmock && !windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/devmock"
)

func systemExecutable(name string) (string, error) {
	dir, err := devmock.StateDir()
	if err != nil {
		return "", err
	}
	sysDir := filepath.Join(dir, "system32")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", sysDir, err)
	}
	path := filepath.Join(sysDir, name)
	if _, statErr := os.Stat(path); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", path, statErr)
		}
		if err := createDevmockSystemExecutable(path); err != nil {
			return "", err
		}
	}
	return path, nil
}

// createDevmockSystemExecutable stands in for the real GetSystemDirectory
// binary: no os.WriteFile (forbidden outside internal/storage), and O_EXCL
// makes a concurrent caller racing the same missing path a no-op rather than
// a clobber.
func createDevmockSystemExecutable(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("sync %s: %w (close: %w)", path, err, closeErr)
		}
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
