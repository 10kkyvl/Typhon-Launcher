package library

import (
	"os"
	"path/filepath"
	"testing"
)

func tempGameExe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestAddAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := newServiceAt(path)

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "  ")
	if err != nil {
		t.Fatal(err)
	}
	if game.Title != "game" {
		t.Fatalf("title = %q", game.Title)
	}
	if game.InstallDir != filepath.Dir(exe) {
		t.Fatalf("install dir = %q", game.InstallDir)
	}
	if game.SizeBytes == 0 {
		t.Fatal("size not computed")
	}

	if _, err := s.AddGame(exe, "Duplicate"); err == nil {
		t.Fatal("expected duplicate error")
	}

	reloaded := newServiceAt(path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].ID != game.ID {
		t.Fatalf("reloaded = %+v", reloaded)
	}

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if len(newServiceAt(path).GetInstalledGames()) != 0 {
		t.Fatal("remove not persisted")
	}
}

func TestPlayMissingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := newServiceAt(path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Ghost")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("expected error for missing executable")
	}
}
