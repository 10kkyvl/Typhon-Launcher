//go:build darwin && !devmock

package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/wine"
)

func demoBottle() wine.Bottle {
	return wine.Bottle{Key: "/Users/x/Games/Demo", Name: "B", Path: "/bottles/B", Drive: "t", Games: "/Users/x/Games"}
}

func TestWineStarterFailsWithoutBottle(t *testing.T) {
	starter := wineStarter{lookup: func(string) (wine.Bottle, bool) { return wine.Bottle{}, false }}

	if _, err := starter.start(t.Context(), "/Users/x/Games/Demo/game.exe", nil, "/Users/x/Games/Demo"); err == nil {
		t.Fatal("start without a bottle: want error")
	}
}

func TestWineStarterWaitsForAppearance(t *testing.T) {
	calls := 0
	var launched wine.Cmd
	starter := wineStarter{
		lookup: func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch: func(_ context.Context, _ wine.Bottle, c wine.Cmd) error { launched = c; return nil },
		poll: func(context.Context, wine.Bottle) ([]wine.Process, error) {
			calls++
			if calls < 2 {
				return nil, nil
			}
			return []wine.Process{{PID: 4242, Path: "/Users/x/Games/Demo/game.exe", CreatedAt: time.Unix(1, 0)}}, nil
		},
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	proc, err := starter.start(t.Context(), "/Users/x/Games/Demo/game.exe", []string{"-windowed"}, "/Users/x/Games/Demo")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if proc.pid() != 4242 {
		t.Fatalf("pid = %d, want 4242", proc.pid())
	}
	if launched.Path != `T:\Demo\game.exe` {
		t.Fatalf("launched Path = %q", launched.Path)
	}
	if launched.WorkDir != `T:\Demo` {
		t.Fatalf("launched WorkDir = %q", launched.WorkDir)
	}
	if len(launched.Args) != 1 || launched.Args[0] != "-windowed" {
		t.Fatalf("launched Args = %v", launched.Args)
	}
}

func TestWineStarterTimesOut(t *testing.T) {
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch:  func(context.Context, wine.Bottle, wine.Cmd) error { return nil },
		poll:    func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: 20 * time.Millisecond,
	}

	_, err := starter.start(t.Context(), "/Users/x/Games/Demo/game.exe", nil, "/Users/x/Games/Demo")
	if !errors.Is(err, errGameNotSeen) {
		t.Fatalf("err = %v, want errGameNotSeen", err)
	}
}

func TestWineStarterReportsLaunchFailure(t *testing.T) {
	boom := errors.New("cxstart failed")
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch:  func(context.Context, wine.Bottle, wine.Cmd) error { return boom },
		poll:    func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	if _, err := starter.start(t.Context(), "/Users/x/Games/Demo/game.exe", nil, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the launch error", err)
	}
}

func TestWineProcessKillStopsBottle(t *testing.T) {
	stopped := false
	proc := &wineGameProcess{
		bottle: demoBottle(),
		id:     4242,
		ctx:    t.Context(),
		poll:   func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:   func(wine.Bottle) error { stopped = true; return nil },
		settle: time.Millisecond,
	}

	if err := proc.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if !stopped {
		t.Fatal("kill did not stop the bottle")
	}
}

func TestWineProcessWaitReturnsWhenGone(t *testing.T) {
	proc := &wineGameProcess{
		bottle: demoBottle(),
		id:     4242,
		ctx:    t.Context(),
		poll:   func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:   func(wine.Bottle) error { return nil },
		settle: time.Millisecond,
	}

	done := make(chan error, 1)
	go func() { done <- proc.wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return once the process was gone")
	}
}

// Нативный исполняемый файл бутыля не требует: на macOS в библиотеке может
// лежать и обычная маковая программа, добавленная руками.
func TestWineStarterRunsNativeExecutableDirectly(t *testing.T) {
	looked := false
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { looked = true; return wine.Bottle{}, false },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	proc, err := starter.start(t.Context(), "/usr/bin/true", nil, "")
	if err != nil {
		t.Fatalf("start native executable: %v", err)
	}
	if looked {
		t.Fatal("native executable must not be looked up among bottles")
	}
	if err := proc.wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestIsWindowsExecutable(t *testing.T) {
	for _, path := range []string{"/games/Demo/game.exe", "/games/Demo/Setup.EXE", "/games/Demo/run.bat"} {
		if !isWindowsExecutable(path) {
			t.Fatalf("isWindowsExecutable(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"/usr/bin/yes", "/Applications/Game.app/Contents/MacOS/Game"} {
		if isWindowsExecutable(path) {
			t.Fatalf("isWindowsExecutable(%q) = true, want false", path)
		}
	}
}
