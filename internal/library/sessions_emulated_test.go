package library

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/compat"
	"typhon/internal/crashlog"
)

type quickExitRun struct {
	native bool
	lived  time.Duration
	err    error
	kill   bool
}

func runSession(t *testing.T, s *Service, id string, run quickExitRun) SessionEvent {
	t.Helper()
	clock := newRegClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s.now = clock.now
	proc := &exitProcess{id: 7, exit: make(chan struct{}), err: run.err}
	s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	s.SetEmulatorResolver(func(string) (string, error) { return filepath.Join("emu", "dir", "emulator.exe"), nil })
	events := make(chan SessionEvent, 1)
	prev := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = prev })

	if err := s.PlayGame(id); err != nil {
		t.Fatal(err)
	}
	if run.kill {
		s.mu.Lock()
		s.running[id].stoppedByUser = true
		s.mu.Unlock()
	}
	clock.set(clock.now().Add(run.lived))
	close(proc.exit)
	s.sessionWG.Wait()
	s.wg.Wait()
	return <-events
}

func TestStoppedEventReportsAQuickEmulatorExit(t *testing.T) {
	const accessViolation = int(-1073741819)
	cases := []struct {
		name    string
		run     quickExitRun
		quick   bool
		crashed bool
	}{
		{"exits at once with code 0", quickExitRun{lived: 0}, true, false},
		{"exits just under the limit", quickExitRun{lived: 4 * time.Second}, true, false},
		{"any exit code counts", quickExitRun{lived: 3 * time.Second, err: codeError(accessViolation)}, true, false},
		{"an unknown exit code counts", quickExitRun{lived: 2 * time.Second, err: errors.New("wait failed")}, true, false},
		{"at the limit it is a session", quickExitRun{lived: 5 * time.Second}, false, false},
		{"a longer failure stays a crash", quickExitRun{lived: 5 * time.Second, err: codeError(1)}, false, true},
		{"a real session", quickExitRun{lived: 26 * time.Second}, false, false},
		{"stopped by the user", quickExitRun{lived: 2 * time.Second, kill: true}, false, false},
		{"a native game", quickExitRun{native: true, lived: 2 * time.Second}, false, false},
		{"a native game keeps its crash", quickExitRun{native: true, lived: 2 * time.Second, err: codeError(1)}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			id := emulatedGame(t, s, "game.iso").ID
			if tc.run.native {
				game, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
				if err != nil {
					t.Fatal(err)
				}
				id = game.ID
			}
			ev := runSession(t, s, id, tc.run)
			if ev.EmulatorQuickExit != tc.quick || ev.Crashed != tc.crashed {
				t.Fatalf("event = %+v, want quick=%v crashed=%v", ev, tc.quick, tc.crashed)
			}
			wantName := ""
			if tc.quick {
				wantName = "PCSX2"
			}
			if ev.Emulator != wantName {
				t.Fatalf("emulator = %q, want %q", ev.Emulator, wantName)
			}
			if tc.quick && ev.ReasonCode != "" {
				t.Fatalf("reason = %q on a quick exit", ev.ReasonCode)
			}
		})
	}
}

func TestQuickEmulatorExitIsNotAnalyzedAsACrash(t *testing.T) {
	cases := []struct {
		name     string
		lived    time.Duration
		analyzed bool
	}{
		{"quick exit", 2 * time.Second, false},
		{"after the limit", 10 * time.Second, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			game := emulatedGame(t, s, "game.iso")
			var emitted atomic.Int32
			hooks := noJournalHooks()
			hooks.await = func(context.Context, crashlog.Query, crashlog.Options) (crashlog.Fault, bool, error) {
				return crashlog.Fault{}, true, nil
			}
			hooks.emit = func(CrashEvent) { emitted.Add(1) }
			s.crash = hooks

			runSession(t, s, game.ID, quickExitRun{lived: tc.lived, err: codeError(1)})

			if got := emitted.Load() == 1; got != tc.analyzed {
				t.Fatalf("crash events = %d, want analyzed=%v", emitted.Load(), tc.analyzed)
			}
		})
	}
}

func TestEmulatedGameStaysOutOfTheCompatJournal(t *testing.T) {
	failed := errors.New("boom")
	cases := []struct {
		name       string
		native     bool
		fail       func(s *Service)
		wantState  compat.State
		wantRecord bool
	}{
		{name: "short sessions", wantState: compat.StateUnknown},
		{name: "start failures", fail: func(s *Service) {
			s.start = func(context.Context, launch) (gameProcess, error) { return nil, failed }
		}, wantState: compat.StateUnknown},
		{name: "runtime failures", fail: func(s *Service) {
			s.prepare = func(context.Context, launch) error { return failed }
		}, wantState: compat.StateUnknown},
		{name: "a native game is still judged", native: true, wantState: compat.StateBroken, wantRecord: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			journal, err := compat.NewServiceAt(filepath.Join(t.TempDir(), "compat.json"))
			if err != nil {
				t.Fatal(err)
			}
			s := newLibrary(t)
			s.SetOutcomeRecorder(journal.RecordSession)
			s.SetLaunchFailureRecorder(journal.RecordLaunchFailure)
			id := emulatedGame(t, s, "game.iso").ID
			if tc.native {
				game, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
				if err != nil {
					t.Fatal(err)
				}
				id = game.ID
			}

			for range 3 {
				if tc.fail == nil {
					runSession(t, s, id, quickExitRun{lived: 26 * time.Second})
					continue
				}
				s.SetEmulatorResolver(func(string) (string, error) { return filepath.Join("emu", "emulator.exe"), nil })
				tc.fail(s)
				if err := s.PlayGame(id); err == nil {
					t.Fatal("the launch was expected to fail")
				}
				s.wg.Wait()
			}

			if got := journal.Status(id).State; got != tc.wantState {
				t.Fatalf("state = %q, want %q", got, tc.wantState)
			}
			if got := len(journal.All()) == 1; got != tc.wantRecord {
				t.Fatalf("journal = %+v, want a record: %v", journal.All(), tc.wantRecord)
			}
		})
	}
}
