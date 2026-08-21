package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := newServiceAt(""); err == nil {
		t.Fatal("empty path must not produce a service")
	}
}

func TestCorruptLibraryFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"id":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := newServiceAt(path); err == nil {
				t.Fatal("corrupt library must not produce a service")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingLibraryStartsEmptyAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if len(s.GetInstalledGames()) != 0 {
		t.Fatalf("games = %+v, want none", s.GetInstalledGames())
	}
	if _, err := s.AddGame(tempGameExe(t), "Game"); err != nil {
		t.Fatalf("add game: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("library not saved: %v", err)
	}
}

func TestRegisterInstalledRejectsEmptyInstallDir(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)
	if _, err := s.RegisterInstalled(InstalledGame{Executable: exe, Title: "Game"}); err == nil {
		t.Fatal("empty install dir must be rejected")
	}
	if games := s.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v, want none", games)
	}
}

func TestApplyInstalledUpdateRejectsEmptyInstallDir(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Game")
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	s.mu.Lock()
	s.games[0].InstallDir = ""
	s.mu.Unlock()
	if _, err := s.ApplyInstalledUpdate(InstalledUpdate{ID: game.ID, Version: "2.0"}); err == nil {
		t.Fatal("empty install dir must be rejected")
	}
}
