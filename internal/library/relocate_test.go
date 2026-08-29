package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func addRelocateGame(t *testing.T, s *Service) (Game, string) {
	t.Helper()
	oldDir := filepath.Join(t.TempDir(), "old")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(oldDir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	game, err := s.AddGame(exe, "Game")
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return game, oldDir
}

func TestRelocateRebasesExecutableAndSaves(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, oldDir := addRelocateGame(t, s)

	savesDir := filepath.Join(oldDir, "saves")
	s.mu.Lock()
	s.games[0].SavesDir = savesDir
	s.mu.Unlock()

	newDir := filepath.Join(t.TempDir(), "new")
	updated, err := s.Relocate(game.ID, newDir)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	wantExe := filepath.Join(newDir, "game.exe")
	if updated.Executable != wantExe {
		t.Fatalf("executable = %q, want %q", updated.Executable, wantExe)
	}
	wantSaves := filepath.Join(newDir, "saves")
	if updated.SavesDir != wantSaves {
		t.Fatalf("savesDir = %q, want %q", updated.SavesDir, wantSaves)
	}
	if updated.InstallDir != newDir {
		t.Fatalf("installDir = %q, want %q", updated.InstallDir, newDir)
	}

	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.InstallDir != newDir {
		t.Fatalf("persisted installDir = %q, want %q", stored.InstallDir, newDir)
	}
}

func TestRelocateKeepsSavesOutsideInstallDir(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _ := addRelocateGame(t, s)

	external := filepath.Join(t.TempDir(), "appdata-saves")
	s.mu.Lock()
	s.games[0].SavesDir = external
	s.mu.Unlock()

	newDir := filepath.Join(t.TempDir(), "new")
	updated, err := s.Relocate(game.ID, newDir)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if updated.SavesDir != external {
		t.Fatalf("savesDir rebased to %q, want unchanged %q", updated.SavesDir, external)
	}
}

func TestRelocateRejectsExecutableOutside(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _ := addRelocateGame(t, s)

	outside := filepath.Join(t.TempDir(), "elsewhere", "game.exe")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.games[0].Executable = outside
	s.mu.Unlock()

	newDir := filepath.Join(t.TempDir(), "new")
	if _, err := s.Relocate(game.ID, newDir); !errors.Is(err, ErrExecutableOutside) {
		t.Fatalf("err = %v, want ErrExecutableOutside", err)
	}

	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.InstallDir == newDir {
		t.Fatal("installDir changed despite rejected relocate")
	}
}

func TestRelocateRollsBackOnPersistFailure(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _ := addRelocateGame(t, s)

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocked, "library.json")

	newDir := filepath.Join(t.TempDir(), "new")
	if _, err := s.Relocate(game.ID, newDir); err == nil {
		t.Fatal("expected persist to fail")
	}

	s.mu.Lock()
	got := s.games[0]
	s.mu.Unlock()
	if got.InstallDir != game.InstallDir {
		t.Fatalf("installDir rolled forward despite failed persist: %q", got.InstallDir)
	}
	if got.Executable != game.Executable {
		t.Fatalf("executable rolled forward despite failed persist: %q", got.Executable)
	}
}

func TestRelocateRejectsEmptyAndRelativeTarget(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _ := addRelocateGame(t, s)

	if _, err := s.Relocate(game.ID, ""); err == nil {
		t.Fatal("empty target must be rejected")
	}
	if _, err := s.Relocate(game.ID, "relative/path"); err == nil {
		t.Fatal("relative target must be rejected")
	}
}
