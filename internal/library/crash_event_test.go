package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/crashlog"
)

func noJournalHooks() crashHooks {
	return crashHooks{
		await: func(context.Context, crashlog.Query, crashlog.Options) (crashlog.Fault, bool, error) {
			return crashlog.Fault{}, false, crashlog.ErrUnsupported
		},
		dumps: func(context.Context, []crashlog.DumpDir, string, time.Time, time.Time) (bool, error) {
			return false, nil
		},
		dir:  func() (string, error) { return "", crashlog.ErrUnsupported },
		emit: func(CrashEvent) {},
	}
}

type journalStub struct {
	fault   crashlog.Fault
	found   bool
	err     error
	queries []crashlog.Query
	opts    []crashlog.Options
	dump    bool
}

func (j *journalStub) hooks() crashHooks {
	h := noJournalHooks()
	h.await = func(_ context.Context, q crashlog.Query, opt crashlog.Options) (crashlog.Fault, bool, error) {
		j.queries = append(j.queries, q)
		j.opts = append(j.opts, opt)
		return j.fault, j.found, j.err
	}
	h.dumps = func(context.Context, []crashlog.DumpDir, string, time.Time, time.Time) (bool, error) {
		return j.dump, nil
	}
	return h
}

func crashGame(t *testing.T) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "Game.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func TestAnalyzeCrashDecision(t *testing.T) {
	dir, exe := crashGame(t)
	base := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	dllNotFound := int(-1073741515)
	fault := crashlog.Fault{Source: crashlog.SourceAppError, App: "Game.exe", Module: "MSVCP140.dll", ExceptionCode: 0xC0000005, HasException: true}
	boom := errors.New("access denied")

	cases := []struct {
		name    string
		exit    *exitInfo
		seconds int64
		j       journalStub
		emit    bool
		check   func(t *testing.T, ev CrashEvent)
	}{
		{"short nonzero exit without journal event", &exitInfo{code: dllNotFound, known: true}, 3, journalStub{}, true,
			func(t *testing.T, ev CrashEvent) {
				if ev.Hint != crashlog.HintRuntimes || ev.ReasonCode != "dll_not_found" || !ev.ExitKnown || ev.ExitCode != dllNotFound || ev.JournalUnknown {
					t.Fatalf("%+v", ev)
				}
			}},
		{"long session with journal event", &exitInfo{code: 1, known: true}, 3600, journalStub{fault: fault, found: true}, true,
			func(t *testing.T, ev CrashEvent) {
				if ev.Hint != crashlog.HintVCRedist || ev.FaultModule != "MSVCP140.dll" || ev.ExceptionCode != "0xC0000005" ||
					ev.ReasonCode != "access_violation" || ev.SessionSeconds != 3600 {
					t.Fatalf("%+v", ev)
				}
			}},
		{"os-detected session with journal event", nil, 900, journalStub{fault: fault, found: true, dump: true}, true,
			func(t *testing.T, ev CrashEvent) {
				if ev.ExitKnown || ev.Hint != crashlog.HintVCRedist || !ev.DumpFound {
					t.Fatalf("%+v", ev)
				}
			}},
		{"os-detected session without event", nil, 900, journalStub{}, false, nil},
		{"normal exit without event", &exitInfo{code: 0, known: true}, 5, journalStub{}, false, nil},
		{"long nonzero exit without event", &exitInfo{code: 1, known: true}, 3600, journalStub{}, false, nil},
		{"short nonzero with unreadable journal", &exitInfo{code: 1, known: true}, 3, journalStub{err: boom}, true,
			func(t *testing.T, ev CrashEvent) {
				if !ev.JournalUnknown || ev.Hint != crashlog.HintUnknown {
					t.Fatalf("%+v", ev)
				}
			}},
		{"long session with unreadable journal", &exitInfo{code: 1, known: true}, 3600, journalStub{err: boom}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := crashInput{gameID: "g1", exePath: exe, installDir: dir, exit: tc.exit, seconds: tc.seconds,
				startedAt: base, endedAt: base.Add(time.Duration(tc.seconds) * time.Second)}
			ev, ok := tc.j.hooks().analyze(t.Context(), in)
			if ok != tc.emit {
				t.Fatalf("emit = %v, want %v (%+v)", ok, tc.emit, ev)
			}
			if ok && ev.GameID != "g1" {
				t.Fatalf("game id: %+v", ev)
			}
			if tc.check != nil {
				tc.check(t, ev)
			}
		})
	}
}

func TestAnalyzeCrashChecksTrackedProcessAndWindow(t *testing.T) {
	dir, _ := crashGame(t)
	tracked := filepath.Join(dir, "bin", "Real.exe")
	base := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	j := journalStub{}
	in := crashInput{gameID: "g", exePath: tracked, installDir: dir, exit: &exitInfo{code: 0, known: true}, seconds: 600,
		startedAt: base, endedAt: base.Add(10 * time.Minute)}
	if _, ok := j.hooks().analyze(t.Context(), in); ok {
		t.Fatal("clean exit must not be a crash")
	}
	if len(j.queries) != 1 || j.queries[0].Exe != "Real.exe" {
		t.Fatalf("query must use the tracked process name: %+v", j.queries)
	}
	if !j.queries[0].From.Before(base) || !j.queries[0].To.After(in.endedAt) {
		t.Fatalf("window must cover the session with slack: %+v", j.queries[0])
	}
	if j.opts[0].MaxWait != 0 {
		t.Fatalf("a clean exit must not wait for WER: %+v", j.opts[0])
	}

	j = journalStub{}
	in.exit = nil
	j.hooks().analyze(t.Context(), in)
	if j.opts[0].MaxWait != crashlog.DefaultOptions.MaxWait {
		t.Fatalf("unknown exit must wait for WER: %+v", j.opts[0])
	}
}

func TestAnalyzeCrashAntivirusWhenExeVanished(t *testing.T) {
	dir, exe := crashGame(t)
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	j := journalStub{}
	in := crashInput{gameID: "g", exePath: exe, installDir: dir, exit: &exitInfo{code: 1, known: true}, seconds: 2,
		startedAt: base, endedAt: base.Add(2 * time.Second)}
	ev, ok := j.hooks().analyze(t.Context(), in)
	if !ok || ev.Hint != crashlog.HintAntivirus || ev.MissingFile != "Game.exe" {
		t.Fatalf("%v %+v", ok, ev)
	}
}

func TestAnalyzeCrashCancelledContextEmitsNothing(t *testing.T) {
	_, exe := crashGame(t)
	h := noJournalHooks()
	h.await = func(ctx context.Context, _ crashlog.Query, _ crashlog.Options) (crashlog.Fault, bool, error) {
		return crashlog.Fault{}, false, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	base := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	in := crashInput{gameID: "g", exePath: exe, exit: &exitInfo{code: 1, known: true}, seconds: 2, startedAt: base, endedAt: base}
	if ev, ok := h.analyze(ctx, in); ok {
		t.Fatalf("shutdown must not produce a crash event: %+v", ev)
	}
}

func TestFinishedSessionEmitsCrashEvent(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		kill    bool
		lived   time.Duration
		journal bool
		emit    bool
	}{
		{"short crash", codeError(1), false, 2 * time.Second, false, true},
		{"long crash found in journal", codeError(1), false, 5 * time.Minute, true, true},
		{"stopped by user", codeError(1), true, 2 * time.Second, true, false},
		{"normal exit", nil, false, 5 * time.Minute, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, err := s.AddGame(t.Context(), tempGameExe(t), "Crash")
			if err != nil {
				t.Fatal(err)
			}
			clock := newRegClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			s.now = clock.now
			j := journalStub{found: tc.journal, fault: crashlog.Fault{App: "game.exe", Module: "d3dx9_43.dll"}}
			got := make(chan CrashEvent, 1)
			s.crash = j.hooks()
			s.crash.emit = func(ev CrashEvent) { got <- ev }
			proc := &exitProcess{id: 9, exit: make(chan struct{}), err: tc.err}
			s.start = func(_ context.Context, _ launch) (gameProcess, error) { return proc, nil }
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			s.AddSessionWatcher(watcher)
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
			s.wg.Wait()
			select {
			case ev := <-got:
				if !tc.emit {
					t.Fatalf("unexpected crash event %+v", ev)
				}
				if ev.GameID != game.ID {
					t.Fatalf("event %+v", ev)
				}
			default:
				if tc.emit {
					t.Fatal("crash event was not emitted")
				}
			}
			if tc.kill && len(j.queries) != 0 {
				t.Fatal("a user-stopped session must not query the journal")
			}
		})
	}
}

func TestAnalyzeCrashDoesNotBorrowAnotherInstancesEvent(t *testing.T) {
	dir, exe := crashGame(t)
	base := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	other := crashlog.Fault{Source: crashlog.SourceAppError, App: "Game.exe", Module: "x.dll", PID: 100, HasPID: true, Time: base.Add(time.Minute)}
	cases := []struct {
		name  string
		exit  *exitInfo
		pid   uint32
		fault crashlog.Fault
		emit  bool
	}{
		{"clean exit after a restart, event of the first run", &exitInfo{code: 0, known: true}, 200, other, false},
		{"clean exit, event without pid", &exitInfo{code: 0, known: true}, 200, crashlog.Fault{App: "Game.exe", Source: crashlog.SourceWER}, false},
		{"clean exit, same pid", &exitInfo{code: 0, known: true}, 100, other, true},
		{"unknown exit, same pid", nil, 100, other, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := noJournalHooks()
			h.await = func(_ context.Context, q crashlog.Query, _ crashlog.Options) (crashlog.Fault, bool, error) {
				f, ok := crashlog.Pick([]crashlog.Fault{tc.fault}, q)
				if q.From.Before(base.Add(-3*time.Second)) || !q.From.Before(base) {
					t.Errorf("window must start just before the session: %v", q.From)
				}
				return f, ok, nil
			}
			in := crashInput{gameID: "g", exePath: exe, installDir: dir, pid: tc.pid, exit: tc.exit, seconds: 600,
				startedAt: base, endedAt: base.Add(10 * time.Minute)}
			if _, ok := h.analyze(t.Context(), in); ok != tc.emit {
				t.Fatalf("emit = %v, want %v", ok, tc.emit)
			}
		})
	}
}

func TestCrashEventNotEmittedAfterShutdown(t *testing.T) {
	_, exe := crashGame(t)
	ctx, cancel := context.WithCancel(t.Context())
	h := noJournalHooks()
	h.await = func(context.Context, crashlog.Query, crashlog.Options) (crashlog.Fault, bool, error) {
		cancel()
		return crashlog.Fault{App: "Game.exe"}, true, nil
	}
	emitted := false
	h.emit = func(CrashEvent) { emitted = true }
	s := &Service{ctx: ctx, crash: h, running: map[string]*session{}, now: time.Now}
	s.running["g"] = &session{exePath: exe, startedAt: time.Now().Add(-time.Minute)}
	s.finishSession(s.ctx, "g", time.Now().Add(-time.Minute))
	s.wg.Wait()
	if emitted {
		t.Fatal("crash event emitted after the service context was cancelled")
	}
}
