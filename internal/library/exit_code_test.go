package library

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type exitProcess struct {
	id   int
	exit chan struct{}
	err  error
}

func (p *exitProcess) pid() int    { return p.id }
func (p *exitProcess) kill() error { return nil }
func (p *exitProcess) wait() error { <-p.exit; return p.err }

type codeError int

func (c codeError) Error() string { return "exit" }
func (c codeError) ExitCode() int { return int(c) }

func TestFinishedSessionCarriesTheExitCode(t *testing.T) {
	const dllNotFound = int(-1073741515)
	cases := []struct {
		name      string
		err       error
		lived     time.Duration
		kill      bool
		known     bool
		crashed   bool
		code      int
		hasReason bool
	}{
		{"short with code", codeError(dllNotFound), 2 * time.Second, false, true, true, dllNotFound, true},
		{"short with zero", nil, 2 * time.Second, false, true, false, 0, false},
		{"long with code", codeError(dllNotFound), 5 * time.Minute, false, true, false, dllNotFound, false},
		{"stopped by user", codeError(1), 2 * time.Second, true, true, false, 1, false},
		{"code unknown", errors.New("wait failed"), 2 * time.Second, false, false, false, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, err := s.AddGame(t.Context(), tempGameExe(t), "Exit")
			if err != nil {
				t.Fatal(err)
			}
			clock := newRegClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			s.now = clock.now
			proc := &exitProcess{id: 5, exit: make(chan struct{}), err: tc.err}
			s.start = func(_ context.Context, _ launch) (gameProcess, error) { return proc, nil }
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			s.AddSessionWatcher(watcher)
			events := make(chan SessionEvent, 1)
			prev := emitStopped
			emitStopped = func(ev SessionEvent) { events <- ev }
			t.Cleanup(func() { emitStopped = prev })

			if err := s.PlayGame(game.ID); err != nil {
				t.Fatal(err)
			}
			if tc.kill {
				s.mu.Lock()
				s.running[game.ID].stoppedByUser = true
				s.mu.Unlock()
			}
			clock.set(clock.now().Add(tc.lived))
			close(proc.exit)
			waitSessionEnd(t, s, watcher)
			ev := <-events
			if ev.ExitKnown != tc.known || ev.Crashed != tc.crashed || ev.ExitCode != tc.code || (ev.ReasonCode != "") != tc.hasReason {
				t.Fatalf("event = %+v, want known=%v crashed=%v code=%d reason=%v", ev, tc.known, tc.crashed, tc.code, tc.hasReason)
			}
		})
	}
}

func TestCrashReasonCodes(t *testing.T) {
	cases := map[int]string{
		-1073741515:     "dll_not_found",
		int(0xC0000135): "dll_not_found",
		-1073741819:     "access_violation",
		-1073741502:     "dll_init_failed",
		-1073741701:     "bad_image",
		-1073741571:     "stack_overflow",
		-532462766:      "dotnet_exception",
		int(0xE0434352): "dotnet_exception",
		7:               "",
	}
	for code, want := range cases {
		if got := crashReasonCode(code); got != want {
			t.Errorf("crashReasonCode(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestDetectTickWaitsForTheExitCodeOfALaunchedGame(t *testing.T) {
	f := newDetectFixture(t)
	proc := &exitProcess{id: 4100, exit: make(chan struct{}), err: codeError(int(0xC0000135))}
	f.s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	f.s.mu.Lock()
	f.s.watching = true
	f.s.mu.Unlock()
	events := make(chan SessionEvent, 1)
	prev := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = prev })

	if err := f.s.PlayGame(f.game.ID); err != nil {
		t.Fatal(err)
	}
	f.scanReturns(nil, true)
	f.clock.set(f.base.Add(2 * time.Second))

	ticked := make(chan struct{})
	go func() {
		defer close(ticked)
		f.s.detectTick(context.Background())
	}()
	close(proc.exit)
	<-ticked
	f.s.sessionWG.Wait()
	if f.s.IsRunning(f.game.ID) {
		f.s.detectTick(context.Background())
	}

	select {
	case ev := <-events:
		if !ev.ExitKnown || !ev.Crashed || ev.ReasonCode != "dll_not_found" {
			t.Fatalf("event = %+v, want a crash with a known exit code", ev)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("session never closed")
	}
}

func TestDetectTickClosesAfterOneDeferralWhenTheExitNeverArrives(t *testing.T) {
	f := newDetectFixture(t)
	proc := &exitProcess{id: 4101, exit: make(chan struct{})}
	f.s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	f.s.mu.Lock()
	f.s.watching = true
	f.s.mu.Unlock()
	events := make(chan SessionEvent, 1)
	prev := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = prev })

	if err := f.s.PlayGame(f.game.ID); err != nil {
		t.Fatal(err)
	}
	f.scanReturns(nil, true)
	f.s.detectTick(context.Background())
	if !f.s.IsRunning(f.game.ID) {
		t.Fatal("first tick must defer the close")
	}
	f.s.detectTick(context.Background())
	if f.s.IsRunning(f.game.ID) {
		t.Fatal("second tick must close the session even without an exit code")
	}
	if ev := <-events; ev.ExitKnown {
		t.Fatalf("event = %+v, want unknown exit", ev)
	}
	close(proc.exit)
	f.s.sessionWG.Wait()
}
