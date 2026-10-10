package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/uierr"
	"typhon/internal/usagestats"
)

type regClock struct{ nanos atomic.Int64 }

func newRegClock(at time.Time) *regClock {
	c := &regClock{}
	c.set(at)
	return c
}

func (c *regClock) now() time.Time { return time.Unix(0, c.nanos.Load()).UTC() }

func (c *regClock) set(at time.Time) { c.nanos.Store(at.UnixNano()) }

type regProcess struct {
	id      int
	exit    chan struct{}
	once    sync.Once
	killErr error
	kills   atomic.Int32
}

func newRegProcess(id int) *regProcess { return &regProcess{id: id, exit: make(chan struct{})} }

func (p *regProcess) pid() int { return p.id }

func (p *regProcess) wait() error {
	<-p.exit
	return nil
}

func (p *regProcess) kill() error {
	p.kills.Add(1)
	if p.killErr != nil {
		return p.killErr
	}
	p.exitNow()
	return nil
}

func (p *regProcess) exitNow() { p.once.Do(func() { close(p.exit) }) }

func waitSessionEnd(t *testing.T, s *Service, w recordingWatcher) {
	t.Helper()
	select {
	case <-w.stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("session never finished")
	}
	s.sessionWG.Wait()
	s.wg.Wait()
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPlayStopRecordsPlaytimeThatSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddGame(tempGameExe(t), "Playtime")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	clock := newRegClock(base)
	s.now = clock.now

	procs := []*regProcess{newRegProcess(101), newRegProcess(102)}
	var launches []launch
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		launches = append(launches, req)
		return procs[len(launches)-1], nil
	}

	type play struct {
		id         string
		start, end time.Time
	}
	type outcome struct {
		played        time.Duration
		stoppedByUser bool
	}
	var (
		mu       sync.Mutex
		plays    []play
		seconds  []int64
		outcomes []outcome
	)
	s.SetPlayRecorder(func(id string, startedAt, endedAt time.Time) {
		mu.Lock()
		defer mu.Unlock()
		plays = append(plays, play{id, startedAt, endedAt})
	})
	s.SetOnSessionEnded(func(_ string, sec int64) {
		mu.Lock()
		defer mu.Unlock()
		seconds = append(seconds, sec)
	})
	s.SetOutcomeRecorder(func(_ string, played time.Duration, byUser bool) {
		mu.Lock()
		defer mu.Unlock()
		outcomes = append(outcomes, outcome{played, byUser})
	})
	watcher := recordingWatcher{started: make(chan Game, 2), stopped: make(chan string, 2)}
	s.AddSessionWatcher(watcher)

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("first PlayGame: %v", err)
	}
	if !s.IsRunning(game.ID) || !slices.Contains(s.GetRunningGames(), game.ID) {
		t.Fatal("started game is not reported as running")
	}
	select {
	case started := <-watcher.started:
		if started.ID != game.ID {
			t.Fatalf("SessionStarted for %q, want %q", started.ID, game.ID)
		}
	default:
		t.Fatal("watcher did not see the start")
	}
	if len(launches) != 1 || launches[0].executable != game.Executable || launches[0].workDir != filepath.Dir(game.Executable) {
		t.Fatalf("launch = %+v, want the game executable in its own directory", launches)
	}

	if err := s.PlayGame(game.ID); uierr.Code(err) != "library.already_running" {
		t.Fatalf("second PlayGame error = %v, want library.already_running", err)
	}
	if len(launches) != 1 {
		t.Fatalf("a running game was launched again: %d launches", len(launches))
	}

	clock.set(base.Add(95 * time.Second))
	if err := s.StopGame(game.ID); err != nil {
		t.Fatalf("StopGame: %v", err)
	}
	waitSessionEnd(t, s, watcher)

	if procs[0].kills.Load() != 1 {
		t.Fatalf("kills = %d, want the stop to kill the game exactly once", procs[0].kills.Load())
	}
	if s.IsRunning(game.ID) {
		t.Fatal("game still running after the session closed")
	}
	got, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaytimeSeconds != 95 {
		t.Fatalf("PlaytimeSeconds = %d, want 95", got.PlaytimeSeconds)
	}
	if got.LastPlayed == nil || !got.LastPlayed.Equal(base.Add(95*time.Second)) {
		t.Fatalf("LastPlayed = %v, want %v", got.LastPlayed, base.Add(95*time.Second))
	}
	reloaded, err := mustServiceAt(t, path).Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.PlaytimeSeconds != 95 || reloaded.LastPlayed == nil || !reloaded.LastPlayed.Equal(base.Add(95*time.Second)) {
		t.Fatalf("playtime lost on reload: %+v", reloaded)
	}
	mu.Lock()
	if len(plays) != 1 || plays[0].id != game.ID || !plays[0].start.Equal(base) || !plays[0].end.Equal(base.Add(95*time.Second)) {
		t.Fatalf("play log = %+v, want one record base..base+95s", plays)
	}
	if !slices.Equal(seconds, []int64{95}) {
		t.Fatalf("session-ended callback seconds = %v, want [95]", seconds)
	}
	if len(outcomes) != 1 || outcomes[0].played != 95*time.Second || !outcomes[0].stoppedByUser {
		t.Fatalf("outcomes = %+v, want 95s stopped by the user", outcomes)
	}
	mu.Unlock()

	clock.set(base.Add(time.Hour))
	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("second session PlayGame: %v", err)
	}
	clock.set(base.Add(time.Hour + 30*time.Second))
	procs[1].exitNow()
	waitSessionEnd(t, s, watcher)

	got, err = s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaytimeSeconds != 125 {
		t.Fatalf("PlaytimeSeconds = %d, want 95 + 30 accumulated across sessions", got.PlaytimeSeconds)
	}
	reloaded, err = mustServiceAt(t, path).Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.PlaytimeSeconds != 125 || reloaded.LastPlayed == nil || !reloaded.LastPlayed.Equal(base.Add(time.Hour+30*time.Second)) {
		t.Fatalf("second session lost on reload: %+v", reloaded)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(outcomes) != 2 || outcomes[1].played != 30*time.Second || outcomes[1].stoppedByUser {
		t.Fatalf("outcomes = %+v, want the second session to be a game exit, not a user stop", outcomes)
	}
	if len(plays) != 2 || !plays[1].start.Equal(base.Add(time.Hour)) {
		t.Fatalf("play log = %+v, want a second record starting at base+1h", plays)
	}
}

func TestPlayGameRefusalStartsNothing(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		arrange func(t *testing.T, s *Service, game Game) string
	}{
		{
			name: "unknown game",
			code: "library.game_not_found",
			arrange: func(*testing.T, *Service, Game) string {
				return "no-such-game"
			},
		},
		{
			name: "uninstalled game",
			code: "library.not_installed",
			arrange: func(t *testing.T, s *Service, game Game) string {
				if err := s.MarkUninstalled(game.ID); err != nil {
					t.Fatal(err)
				}
				return game.ID
			},
		},
		{
			name: "game without an executable",
			code: "library.executable_missing",
			arrange: func(_ *testing.T, s *Service, game Game) string {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.findLocked(game.ID).Executable = ""
				return game.ID
			},
		},
		{
			name: "executable deleted from disk",
			code: "library.executable_missing",
			arrange: func(t *testing.T, _ *Service, game Game) string {
				if err := os.Remove(game.Executable); err != nil {
					t.Fatal(err)
				}
				return game.ID
			},
		},
		{
			name: "service shut down",
			code: "library.not_running",
			arrange: func(t *testing.T, s *Service, game Game) string {
				if err := s.ServiceShutdown(); err != nil {
					t.Fatal(err)
				}
				return game.ID
			},
		},
		{
			name: "service never started",
			code: "library.cannot_confirm_process",
			arrange: func(_ *testing.T, s *Service, game Game) string {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.ctx = nil
				return game.ID
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			game, err := s.AddGame(tempGameExe(t), "Refused")
			if err != nil {
				t.Fatal(err)
			}
			var started, prepared int
			s.start = func(context.Context, launch) (gameProcess, error) {
				started++
				return newRegProcess(1), nil
			}
			s.prepare = func(context.Context, launch) error {
				prepared++
				return nil
			}
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			s.AddSessionWatcher(watcher)
			var usageMu sync.Mutex
			var usage []usagestats.Event
			s.SetUsageRecorder(func(ev usagestats.Event) {
				usageMu.Lock()
				defer usageMu.Unlock()
				usage = append(usage, ev)
			})

			id := tc.arrange(t, s, game)
			before := readFile(t, path)

			err = s.PlayGame(id)
			if err == nil {
				t.Fatal("PlayGame succeeded, want a refusal")
			}
			if got := uierr.Code(err); got != tc.code {
				t.Fatalf("error code = %q (%v), want %q", got, err, tc.code)
			}
			if started != 0 || prepared != 0 {
				t.Fatalf("refused game reached the platform layer: start=%d prepare=%d", started, prepared)
			}
			if s.IsRunning(id) || len(s.GetRunningGames()) != 0 {
				t.Fatal("refused game is reported as running")
			}
			select {
			case g := <-watcher.started:
				t.Fatalf("SessionStarted for a refused game: %+v", g)
			default:
			}
			usageMu.Lock()
			defer usageMu.Unlock()
			if len(usage) != 0 {
				t.Fatalf("usage events for a refused game: %+v", usage)
			}
			if after := readFile(t, path); !bytes.Equal(before, after) {
				t.Fatal("a refused launch rewrote the library file")
			}
		})
	}
}

func TestStopGameRefusesWhatIsNotRunning(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Idle")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{game.ID, "no-such-game", ""} {
		if err := s.StopGame(id); uierr.Code(err) != "library.not_running" {
			t.Fatalf("StopGame(%q) = %v, want library.not_running", id, err)
		}
	}
}

func TestStopGameKillFailureKeepsTheSessionTracked(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Stubborn")
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("access denied")
	proc := newRegProcess(77)
	proc.killErr = denied
	s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.StopGame(game.ID); !errors.Is(err, denied) {
		t.Fatalf("StopGame error = %v, want the kill failure to reach the caller", err)
	}
	if !s.IsRunning(game.ID) {
		t.Fatal("a game that could not be killed was dropped from the running set")
	}

	proc.exitNow()
	waitSessionEnd(t, s, watcher)
	if s.IsRunning(game.ID) {
		t.Fatal("session did not close once the game exited by itself")
	}
}

func TestServiceShutdownCancelsALaunchThatIsStillStarting(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Waiting for UAC")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	s.start = func(ctx context.Context, _ launch) (gameProcess, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	played := make(chan error, 1)
	go func() { played <- s.PlayGame(game.ID) }()
	<-entered
	if !s.IsRunning(game.ID) {
		t.Fatal("a game that is still starting must be reported as busy")
	}

	shutdown := make(chan error, 1)
	go func() { shutdown <- s.ServiceShutdown() }()

	select {
	case err := <-played:
		if uierr.Code(err) != "library.launch_cancelled" || !errors.Is(err, context.Canceled) {
			t.Fatalf("PlayGame error = %v, want a coded cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("PlayGame did not return after shutdown cancelled the launch")
	}
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServiceShutdown hung on a pending launch")
	}
	if s.IsRunning(game.ID) {
		t.Fatal("cancelled launch left the game marked as running")
	}
}
