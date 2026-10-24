package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type hookFunc func(ctx context.Context, game Game) error

func (f hookFunc) SessionStarting(ctx context.Context, game Game) error { return f(ctx, game) }

func hookedGame(t *testing.T) (*Service, string, *[]string) {
	t.Helper()
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGame(t.Context(), exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	s.ctx = t.Context()
	s.prepare = func(context.Context, launch) error { order = append(order, "prepare"); return nil }
	s.start = func(context.Context, launch) (gameProcess, error) {
		order = append(order, "start")
		return nil, errors.New("stop before a real process")
	}
	return s, g.ID, &order
}

func TestLaunchHookRunsBetweenPrepareAndStartOutsideTheLock(t *testing.T) {
	s, id, order := hookedGame(t)
	var seen Game
	locked := false
	s.AddLaunchHook(hookFunc(func(_ context.Context, g Game) error {
		seen = g
		*order = append(*order, "hook")
		locked = !s.mu.TryLock()
		if !locked {
			s.mu.Unlock()
		}
		return nil
	}))

	if err := s.PlayGame(id); err == nil {
		t.Fatal("expected the injected start failure")
	}

	if got := *order; len(got) != 3 || got[0] != "prepare" || got[1] != "hook" || got[2] != "start" {
		t.Fatalf("order = %v, want prepare, hook, start", got)
	}
	if locked {
		t.Fatal("library mutex held while the hook ran")
	}
	if seen.ID != id || seen.Executable == "" {
		t.Fatalf("hook got %+v, want the launching game with its executable", seen)
	}
}

func TestLaunchHookFailureDoesNotBlockTheLaunch(t *testing.T) {
	s, id, order := hookedGame(t)
	s.AddLaunchHook(hookFunc(func(context.Context, Game) error { return errors.New("disk full") }))

	if err := s.PlayGame(id); err == nil {
		t.Fatal("expected the injected start failure")
	}

	if got := *order; len(got) != 2 || got[1] != "start" {
		t.Fatalf("order = %v, want start to run after a failed hook", got)
	}
}

func TestLaunchHookSeesCancellationAndStopsTheLaunch(t *testing.T) {
	s, id, order := hookedGame(t)
	entered := make(chan struct{})
	s.AddLaunchHook(hookFunc(func(ctx context.Context, _ Game) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}))
	done := make(chan error, 1)
	go func() { done <- s.PlayGame(id) }()
	<-entered

	if err := s.StopGame(id); err != nil {
		t.Fatal(err)
	}

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("PlayGame = %v, want a cancelled launch", err)
	}
	for _, step := range *order {
		if step == "start" {
			t.Fatalf("order = %v, process was started after the player cancelled", *order)
		}
	}
	if s.IsRunning(id) {
		t.Fatal("cancelled launch kept its reservation")
	}
}
