package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExecutableAbsoluteMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.exe")
	if _, err := resolveExecutable(path); err == nil {
		t.Fatal("expected error for missing absolute path")
	}
}

func TestResolveExecutableAbsoluteDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveExecutable(dir); !errors.Is(err, errNoExecutable) {
		t.Fatalf("resolveExecutable(dir) error = %v, want errNoExecutable", err)
	}
}

func TestResolveExecutableAbsoluteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool.exe")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveExecutable(path)
	if err != nil {
		t.Fatalf("resolveExecutable: %v", err)
	}
	if got != path {
		t.Fatalf("resolveExecutable = %s, want %s", got, path)
	}
}
