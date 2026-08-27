//go:build windows

package shortcut

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndRemove(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "Test Game.lnk")
	link := Link{
		Target:      exe,
		Args:        `--launch "Test Game"`,
		WorkDir:     dir,
		Description: "Test Game shortcut",
	}

	if err := Create(path, link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat created shortcut: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("created shortcut %s is empty", path)
	}

	if err := Create(path, link); err != nil {
		t.Fatalf("second Create (overwrite) failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat overwritten shortcut: %v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("shortcut still present after Remove: err=%v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("second Remove of already-missing file: %v", err)
	}
}

func TestCreateMissingDirectory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	path := filepath.Join(t.TempDir(), "no-such-subdir", "Game.lnk")
	link := Link{Target: exe}

	if err := Create(path, link); err == nil {
		t.Fatalf("Create into missing directory: want error, got nil")
	}
}

func TestCreateEmptyTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.lnk")
	link := Link{Target: ""}

	if err := Create(path, link); err == nil {
		t.Fatalf("Create with empty Target: want error, got nil")
	}
}

func TestSupported(t *testing.T) {
	if !Supported() {
		t.Fatalf("Supported() = false on windows, want true")
	}
}

func TestDesktopDir(t *testing.T) {
	dir, err := DesktopDir()
	if err != nil {
		t.Fatalf("DesktopDir: %v", err)
	}
	if dir == "" {
		t.Fatalf("DesktopDir returned empty path with nil error")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat desktop dir %s: %v", dir, err)
	}
}
