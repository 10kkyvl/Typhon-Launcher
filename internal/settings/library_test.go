package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLibraryPathDrivesSubfolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	root := filepath.Join(t.TempDir(), LibraryFolderName)
	next := s.GetSettings()
	next.LibraryPath = root
	next.GamesPath = `Z:\somewhere-else`
	next.DownloadsPath = `Z:\somewhere-else`
	next.ScreenshotsPath = `Z:\somewhere-else`
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	got := s.GetSettings()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"games", got.GamesPath, filepath.Join(root, dirGames)},
		{"downloads", got.DownloadsPath, filepath.Join(root, dirDownloads)},
		{"screenshots", got.ScreenshotsPath, filepath.Join(root, dirScreenshots)},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s path = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestUnconfiguredLibraryKeepsPathsEmpty(t *testing.T) {
	got := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json")).GetSettings()
	if got.LibraryPath != "" || got.GamesPath != "" || got.DownloadsPath != "" || got.ScreenshotsPath != "" {
		t.Fatalf("fresh settings must not invent paths: %+v", got)
	}
}

func TestRelativeLibraryPathRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	next := s.GetSettings()
	next.LibraryPath = filepath.Join("relative", LibraryFolderName)
	err := s.SaveSettings(next)
	if !errors.Is(err, ErrLibraryPathRelative) {
		t.Fatalf("err = %v, want ErrLibraryPathRelative", err)
	}
	if got := s.GetSettings().LibraryPath; got != "" {
		t.Fatalf("library path = %q after rejected save, want empty", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected save wrote settings file: %v", err)
	}
}

func TestVolumeRootLibraryPathRejected(t *testing.T) {
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	next := s.GetSettings()
	next.LibraryPath = root
	if err := s.SaveSettings(next); !errors.Is(err, ErrLibraryPathRoot) {
		t.Fatalf("err = %v, want ErrLibraryPathRoot", err)
	}
}

func TestSetupLibraryCreatesLayout(t *testing.T) {
	parent := t.TempDir()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))

	got, err := s.SetupLibrary(parent)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, LibraryFolderName)
	if got.LibraryPath != root {
		t.Fatalf("library path = %q, want %q", got.LibraryPath, root)
	}
	for _, dir := range []string{root, got.GamesPath, got.DownloadsPath, got.ScreenshotsPath} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("library root has %d entries, want 3", len(entries))
	}
}

func TestSetupLibraryReusesSelectedLibraryFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), LibraryFolderName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))

	got, err := s.SetupLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.LibraryPath != root {
		t.Fatalf("library path = %q, want %q", got.LibraryPath, root)
	}
	if _, err := os.Stat(filepath.Join(root, LibraryFolderName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nested library folder created: %v", err)
	}
}

func TestSetupLibraryRejectsBadParent(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		parent string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"relative", filepath.Join("games", "here")},
		{"file", file},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.SetupLibrary(tc.parent); err == nil {
				t.Fatalf("parent %q accepted", tc.parent)
			}
			if got := s.GetSettings().LibraryPath; got != "" {
				t.Fatalf("library path = %q after failed setup, want empty", got)
			}
		})
	}
}

func TestProposeLibraryPath(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	parent := t.TempDir()

	got, err := s.ProposeLibraryPath(parent)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(parent, LibraryFolderName); got != want {
		t.Fatalf("proposed %q, want %q", got, want)
	}
	if _, err := os.Stat(got); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("proposal created the folder: %v", err)
	}
}

func TestLegacyGamesPathBecomesLibrary(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "Typhon", dirGames)
	path := filepath.Join(t.TempDir(), "settings.json")
	data, err := json.Marshal(map[string]string{"theme": "dark", "gamesPath": legacy})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got := mustServiceAt(t, path).GetSettings()
	want := filepath.Join(base, "Typhon")
	if got.LibraryPath != want {
		t.Fatalf("library path = %q, want %q", got.LibraryPath, want)
	}
	if got.GamesPath != legacy {
		t.Fatalf("games path = %q, want %q", got.GamesPath, legacy)
	}
}

func TestLegacyRelativeGamesPathIsNotMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"gamesPath":"Games"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.LibraryPath != "" || got.GamesPath != "" {
		t.Fatalf("relative legacy path leaked: %+v", got)
	}
}
