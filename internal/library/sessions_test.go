package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayGameTracksSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := newServiceAt(path)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Session Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(s.GetRunningGames()) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("session never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if s.GetInstalledGames()[0].LastPlayed == nil {
		t.Fatal("last played not set")
	}
	if newServiceAt(path).GetInstalledGames()[0].LastPlayed == nil {
		t.Fatal("last played not persisted")
	}
}

func TestPlayGameRunsInExecutableDir(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "Game.v1.0")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(gameDir, "game.exe")
	data, err := os.ReadFile(`C:\Windows\System32\cmd.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, data, 0o755); err != nil {
		t.Fatal(err)
	}

	s := newServiceAt(filepath.Join(root, "library.json"))
	game, err := s.AddGame(exe, "Nested Game")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.InstallDir = root
	stored.LaunchArgs = []string{"/C", "cd > cwd.txt"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(s.GetRunningGames()) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("session never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := os.Stat(filepath.Join(gameDir, "cwd.txt")); err != nil {
		t.Fatalf("game did not run in executable dir: %v", err)
	}
}
