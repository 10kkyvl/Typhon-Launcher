//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func SaveRoots() ([]SaveRoot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	return []SaveRoot{
		{Path: filepath.Join(home, ".local", "share"), Depth: 2},
		{Path: filepath.Join(home, ".config"), Depth: 2},
		{Path: filepath.Join(home, "Documents"), Depth: 1},
		{Path: home, Depth: 1},
	}, nil
}
