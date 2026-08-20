package platform

import (
	"os"
	"path/filepath"
)

func nearestExistingDir(path string) string {
	dir := filepath.Clean(path)
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}
