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
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
