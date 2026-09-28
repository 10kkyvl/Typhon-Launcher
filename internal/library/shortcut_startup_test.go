package library

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestPlayGameBeforeServiceStartupFailsThenSucceedsAfter(t *testing.T) {
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.start = execStarter
	s.prepare = func(context.Context, launch) error { return nil }

	exe, exitArgs := testExecutable(t)
	game, err := s.AddGame(exe, "Shortcut Game")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = exitArgs
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); !errors.Is(err, errSessionCannotConfirm) {
		t.Fatalf("PlayGame before ServiceStartup = %v, want errSessionCannotConfirm", err)
	}
	if s.IsRunning(game.ID) {
		t.Fatal("PlayGame opened a session before ServiceStartup ran")
	}

	watcher := recordingWatcher{started: make(chan Game, 2), stopped: make(chan string, 2)}
	s.AddSessionWatcher(watcher)

	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame after ServiceStartup: %v", err)
	}

	select {
	case started := <-watcher.started:
		if started.ID != game.ID {
			t.Fatalf("started = %+v, want game %s", started, game.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("game did not start after ServiceStartup")
	}
	select {
	case started := <-watcher.started:
		t.Fatalf("game started a second time: %+v", started)
	default:
	}
}
