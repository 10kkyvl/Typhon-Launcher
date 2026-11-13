package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
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

func TestLaunchHookBlockedErrorStopsTheLaunch(t *testing.T) {
	s, id, order := hookedGame(t)
	cause := errors.New("rollback unfinished")
	s.AddLaunchHook(hookFunc(func(context.Context, Game) error { return &LaunchBlockedError{Err: cause} }))

	err := s.PlayGame(id)

	if !errors.Is(err, cause) {
		t.Fatalf("PlayGame = %v, want the blocking cause", err)
	}
	for _, step := range *order {
		if step == "start" {
			t.Fatalf("order = %v, process started despite a blocking hook error", *order)
		}
	}
	if s.IsRunning(id) {
		t.Fatal("blocked launch kept its reservation")
	}
	if err := s.PlayGame(id); !errors.Is(err, cause) {
		t.Fatalf("second PlayGame = %v, want the library lock to be released", err)
	}
}

type startWatcher chan string

func (w startWatcher) SessionStarted(g Game) { w <- g.ID }
func (w startWatcher) SessionStopped(string) {}

func TestBlockedLaunchStartsNothingAndIsNotJournaledAsAFailure(t *testing.T) {
	control, controlID, _ := hookedGame(t)
	controlFailures := make(chan string, 4)
	control.SetLaunchFailureRecorder(func(_, code, _ string) { controlFailures <- code })
	control.AddLaunchHook(hookFunc(func(context.Context, Game) error { return nil }))
	if err := control.PlayGame(controlID); err == nil {
		t.Fatal("control: expected the injected start failure")
	}
	if err := control.ServiceShutdown(); err != nil {
		t.Fatalf("control shutdown: %v", err)
	}
	select {
	case code := <-controlFailures:
		if code != "library.launch_failed" {
			t.Fatalf("control: journaled %q, want library.launch_failed", code)
		}
	default:
		t.Fatal("control: a failed start was not journaled, so the check below would prove nothing")
	}

	s, id, order := hookedGame(t)
	failures := make(chan string, 4)
	s.SetLaunchFailureRecorder(func(_, code, _ string) { failures <- code })
	started := make(startWatcher, 4)
	s.AddSessionWatcher(started)
	cause := uierr.New("cloud.test_blocked", "saves unsafe")
	s.AddLaunchHook(hookFunc(func(context.Context, Game) error {
		return &LaunchBlockedError{Err: fmt.Errorf("launch check: %w", cause)}
	}))

	err := s.PlayGame(id)

	if code := uierr.Code(err); code != "cloud.test_blocked" || !errors.Is(err, cause) {
		t.Fatalf("PlayGame = %v (code %q), want the hook's cause and code to reach the caller", err, code)
	}
	for _, step := range *order {
		if step == "start" {
			t.Fatalf("order = %v, process started despite a blocking hook", *order)
		}
	}
	if s.IsRunning(id) || len(s.GetRunningGames()) != 0 {
		t.Fatalf("running or starting %v after a blocked launch", s.GetRunningGames())
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case code := <-failures:
		t.Fatalf("blocked launch journaled as a launch failure %q", code)
	default:
	}
	select {
	case g := <-started:
		t.Fatalf("session watchers were told %s started", g)
	default:
	}
}

func TestBlockingHookKeepsLaterHooksFromRunning(t *testing.T) {
	s, id, order := hookedGame(t)
	cause := errors.New("saves unsafe")
	s.AddLaunchHook(hookFunc(func(context.Context, Game) error {
		*order = append(*order, "first")
		return &LaunchBlockedError{Err: cause}
	}))
	s.AddLaunchHook(hookFunc(func(context.Context, Game) error {
		*order = append(*order, "second")
		return nil
	}))

	if err := s.PlayGame(id); !errors.Is(err, cause) {
		t.Fatalf("PlayGame = %v, want the first hook's cause", err)
	}
	if got := *order; len(got) != 2 || got[0] != "prepare" || got[1] != "first" {
		t.Fatalf("order = %v, want prepare, first: a blocked launch must not run the hooks after the block", got)
	}
}
