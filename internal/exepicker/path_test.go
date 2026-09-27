package exepicker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitialLocationKeepsDirectoryWithDot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Game v1.2")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	gotDir, gotFile := InitialLocation(dir)
	if gotDir != dir || gotFile != "" {
		t.Fatalf("InitialLocation(%q) = %q, %q; want directory unchanged", dir, gotDir, gotFile)
	}
}

func TestInitialLocationPrefillsExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Game.exe")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	gotDir, gotFile := InitialLocation(file)
	if gotDir != dir || gotFile != "Game.exe" {
		t.Fatalf("InitialLocation(%q) = %q, %q", file, gotDir, gotFile)
	}
}
