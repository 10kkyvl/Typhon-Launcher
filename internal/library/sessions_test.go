package library

import (
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
