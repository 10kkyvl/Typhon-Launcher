//go:build windows

package install

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func systemExecutable(name string) (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve system directory: %w", err)
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: %s", errNoExecutable, path)
	}
	return path, nil
}
