//go:build devmock && !windows

package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/devmock"
	"typhon/internal/uierr"
)

func TestDevmockPlayGameRegistersFakeProcess(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	s.start = newGameStarter()

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Devmock Game")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	running := s.GetRunningGames()
	if len(running) != 1 || running[0] != game.ID {
		t.Fatalf("GetRunningGames = %v, want [%s]", running, game.ID)
	}

	entries, err := devmock.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Path == exe {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("devmock.List() = %+v, want an entry for %s", entries, exe)
	}

	if err := s.StopGame(game.ID); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()

	if s.IsRunning(game.ID) {
		t.Fatal("session still running after StopGame")
	}
}

func TestDevmockDetectTickClosesSessionAfterStop(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	s.start = newGameStarter()

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Devmock Detect")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := base
	s.now = func() time.Time { return clock }
	s.watchInterval = time.Second
	// Simulate an active detector without running the real loop, same as
	// TestDetectKeepsLaunchedSessionAliveWhilePathUnverifiable: this is what
	// makes the wait goroutine defer closing the session to detectTick
	// instead of closing it itself the instant the fake process is killed.
	s.watching = true

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.StopGame(game.ID); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()

	if !s.IsRunning(game.ID) {
		t.Fatal("session closed itself despite an active detector")
	}

	// s.scan is procs.List, which under devmock maps devmock.List(): once
	// StopGame's kill removed the fake process from the registry, the next
	// scan no longer reports its pid, and the session must close once the
	// grace period elapses.
	clock = clock.Add(3 * s.watchInterval)
	s.detectTick(context.Background())

	if s.IsRunning(game.ID) {
		t.Fatal("session stayed running after its devmock process was killed and the grace period elapsed")
	}
}

func TestDevmockPlayGameFailsWhenExecutableGone(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	s.start = newGameStarter()

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}

	err = s.PlayGame(game.ID)
	if err == nil || uierr.Code(err) != "library.executable_missing" {
		t.Fatalf("PlayGame error = %v, want code library.executable_missing", err)
	}
}
