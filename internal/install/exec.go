package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func resolveExecutable(path string) (string, error) {
	if filepath.IsAbs(path) {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%w: %s", errNoExecutable, path)
		}
		return path, nil
	}
	found, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errNoExecutable, err)
	}
	return found, nil
}
