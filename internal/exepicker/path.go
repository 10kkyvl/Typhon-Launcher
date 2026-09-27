package exepicker

import (
	"os"
	"path/filepath"
)

// InitialLocation distinguishes files from directories using the filesystem.
// A dot in a directory name is not evidence that the path is a file.
func InitialLocation(path string) (directory, file string) {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return filepath.Dir(path), filepath.Base(path)
	}
	return path, ""
}
