//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func openFolderWindows(path string) error {
	exe, err := explorerPath()
	if err != nil {
		return err
	}
	return exec.Command(exe, path).Start()
}

// explorerPath резолвит абсолютный путь до explorer.exe (инвариант 33):
// голое имя запускалось бы через PATH, который пользователь может подменить.
// explorer.exe лежит в каталоге Windows, а не в System32 (GetSystemDirectory
// вернул бы неверный путь).
func explorerPath() (string, error) {
	dir, err := windows.GetWindowsDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve windows directory: %w", err)
	}
	path := filepath.Join(dir, "explorer.exe")
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file: %s", path)
	}
	return path, nil
}
