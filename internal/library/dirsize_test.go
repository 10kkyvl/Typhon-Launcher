package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirSizeReportsFailures(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "game.exe")
	if err := os.WriteFile(file, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	size, err := dirSize(root)
	if err != nil {
		t.Fatalf("dirSize: %v", err)
	}
	if size != 1024 {
		t.Fatalf("size = %d, want 1024", size)
	}

	cases := []struct{ name, dir string }{
		{name: "empty path", dir: ""},
		{name: "missing dir", dir: filepath.Join(root, "gone")},
		{name: "file on the path", dir: filepath.Join(file, "sub")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := dirSize(tc.dir)
			if err == nil {
				t.Fatalf("dirSize = %d, want error", size)
			}
			if size != 0 {
				t.Fatalf("size = %d, want 0 alongside the error", size)
			}
		})
	}
}

func TestRegisterInstalledMarksUnknownSize(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)

	game, err := s.RegisterInstalled(InstalledGame{
		Executable: exe,
		Title:      "Game",
		InstallDir: filepath.Join(exe, "sub"),
	})
	if err != nil {
		t.Fatalf("RegisterInstalled: %v", err)
	}
	if !game.SizeUnknown {
		t.Fatal("unmeasurable install dir must be marked as unknown size, not as 0 bytes")
	}
	if game.SizeBytes != 0 {
		t.Fatalf("sizeBytes = %d, want 0", game.SizeBytes)
	}
}
