package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayGameTracksSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

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
	if mustServiceAt(t, path).GetInstalledGames()[0].LastPlayed == nil {
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

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
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

type recordingWatcher struct {
	started chan Game
	stopped chan string
}

func (r recordingWatcher) SessionStarted(game Game) { r.started <- game }

func (r recordingWatcher) SessionStopped(gameID string) { r.stopped <- gameID }

func TestSessionWatcherSeesStartAndStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.SetSessionWatcher(watcher)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Watcher Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case started := <-watcher.started:
		if started.ID != game.ID || started.Title != "Watcher Test" {
			t.Fatalf("started = %+v", started)
		}
	default:
		t.Fatal("watcher did not see the start")
	}

	select {
	case stopped := <-watcher.stopped:
		if stopped != game.ID {
			t.Fatalf("stopped = %q", stopped)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("watcher did not see the stop")
	}
}
