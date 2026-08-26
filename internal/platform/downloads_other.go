//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, "Downloads")
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("downloads folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("downloads folder %s: %w", dir, os.ErrInvalid)
	}
	return dir, nil
}
