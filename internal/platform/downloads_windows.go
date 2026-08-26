package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func DownloadsDir() (string, error) {
	path, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("downloads folder: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("downloads folder: %w", ErrEmptyPath)
	}
	return path, nil
}
