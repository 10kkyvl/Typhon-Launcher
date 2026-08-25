package library

import (
	"os"
	"path/filepath"
	"strings"
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

func mustServiceAt(t testing.TB, path string) *Service {
	t.Helper()
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new library service at %s: %v", path, err)
	}
	return s
}

func TestAddAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

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

	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].ID != game.ID {
		t.Fatalf("reloaded = %+v", reloaded)
	}

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if len(mustServiceAt(t, path).GetInstalledGames()) != 0 {
		t.Fatal("remove not persisted")
	}
}

func TestPlayMissingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
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

func TestRegisterInstalledAddsGame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)

	game, err := s.RegisterInstalled(InstalledGame{
		Title:            "  Space Game  ",
		Executable:       exe,
		InstallDir:       filepath.Dir(exe),
		Version:          "1.2.3",
		SourceDownloadID: "d1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if game.Title != "Space Game" || game.Version != "1.2.3" || game.SourceDownloadID != "d1" {
		t.Fatalf("game = %+v", game)
	}
	if game.SizeBytes == 0 || game.InstalledAt.IsZero() {
		t.Fatalf("game = %+v", game)
	}

	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].SourceDownloadID != "d1" {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}

func TestRegisterInstalledUpdatesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)

	first, err := s.AddGame(exe, "Original")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.RegisterInstalled(InstalledGame{
		Executable:       strings.ToUpper(exe),
		InstallDir:       filepath.Dir(exe),
		Version:          "2.0",
		SourceDownloadID: "d2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID {
		t.Fatalf("id = %q, want %q", updated.ID, first.ID)
	}
	if updated.Title != "Original" {
		t.Fatalf("title = %q, want the original one", updated.Title)
	}
	if updated.Version != "2.0" || updated.SourceDownloadID != "d2" {
		t.Fatalf("game = %+v", updated)
	}
	if games := s.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %+v", games)
	}
}

func TestRegisterInstalledRejectsMissingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if _, err := s.RegisterInstalled(InstalledGame{Executable: filepath.Join(t.TempDir(), "nope.exe")}); err == nil {
		t.Fatal("expected error for missing executable")
	}
}
