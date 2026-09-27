package platform

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func OpenFolder(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("folder unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a folder: %s", path)
	}
	switch runtime.GOOS {
	case "windows":
		return openFolderWindows(path)
	case "darwin":
		exe, err := darwinOpenPath()
		if err != nil {
			return err
		}
		return exec.Command(exe, path).Start() //nolint:gosec // G204: путь до бинаря абсолютный и проверен (инвариант 33), path — существующий каталог, проверенный выше через os.Stat+IsDir
	default:
		exe, err := exec.LookPath("xdg-open")
		if err != nil {
			return fmt.Errorf("resolve xdg-open: %w", err)
		}
		return exec.Command(exe, path).Start() //nolint:gosec // G204: путь до бинаря абсолютный и проверен (инвариант 33), path — существующий каталог, проверенный выше через os.Stat+IsDir
	}
}

// darwinOpenPath резолвит абсолютный путь до /usr/bin/open (инвариант 33):
// голое имя запускалось бы через PATH, который пользователь может подменить.
func darwinOpenPath() (string, error) {
	const path = "/usr/bin/open"
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file: %s", path)
	}
	return path, nil
}
