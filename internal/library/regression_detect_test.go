package library

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/procs"
)

type detectFixture struct {
	s     *Service
	game  Game
	dir   string
	exe   string
	clock *regClock
	base  time.Time
}

func newDetectFixture(t *testing.T) *detectFixture {
	t.Helper()
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))
	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Detected"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	clock := newRegClock(base)
	s.now = clock.now
	s.watchInterval = time.Second
	return &detectFixture{s: s, game: game, dir: dir, exe: exe, clock: clock, base: base}
}

func (f *detectFixture) scanReturns(list []procs.Process, complete bool) {
	f.s.scan = func(context.Context) ([]procs.Process, bool, error) { return list, complete, nil }
}

func (f *detectFixture) session(t *testing.T) *session {
	t.Helper()
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	return f.s.running[f.game.ID]
}

func TestDetectedSessionRecordsExactPlaytime(t *testing.T) {
	f := newDetectFixture(t)
	created := f.base.Add(-60 * time.Second)
	f.scanReturns([]procs.Process{{PID: 4001, Path: f.exe, CreatedAt: created}}, true)

	var mu sync.Mutex
	type play struct {
		id         string
		start, end time.Time
	}
	var plays []play
	var seconds []int64
	f.s.SetPlayRecorder(func(id string, startedAt, endedAt time.Time) {
		mu.Lock()
		defer mu.Unlock()
		plays = append(plays, play{id, startedAt, endedAt})
	})
	f.s.SetOnSessionEnded(func(_ string, sec int64) {
		mu.Lock()
		defer mu.Unlock()
		seconds = append(seconds, sec)
	})
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	f.s.AddSessionWatcher(watcher)

	f.s.detectTick(context.Background())
	if !f.s.IsRunning(f.game.ID) {
		t.Fatal("session not opened for the running game")
	}
	select {
	case <-watcher.started:
	default:
		t.Fatal("watcher did not see the detected start")
	}

	f.clock.set(f.base.Add(30 * time.Second))
	f.scanReturns(nil, true)
	f.s.detectTick(context.Background())
	waitSessionEnd(t, f.s, watcher)

	got, err := f.s.Find(f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlaytimeSeconds != 90 {
		t.Fatalf("PlaytimeSeconds = %d, want 90 (60s before detection + 30s after)", got.PlaytimeSeconds)
	}
	if got.LastPlayed == nil || !got.LastPlayed.Equal(f.base.Add(30*time.Second)) {
		t.Fatalf("LastPlayed = %v, want %v", got.LastPlayed, f.base.Add(30*time.Second))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(plays) != 1 || plays[0].id != f.game.ID || !plays[0].start.Equal(created) || !plays[0].end.Equal(f.base.Add(30*time.Second)) {
		t.Fatalf("play log = %+v, want one record from the process start to the close", plays)
	}
	if len(seconds) != 1 || seconds[0] != 90 {
		t.Fatalf("session-ended callback seconds = %v, want [90]", seconds)
	}
}

func TestDetectIgnoresGamesItCannotLocate(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))
	stranger := filepath.Join(root, "Elsewhere", "other.exe")

	tests := []struct {
		name        string
		game        Game
		procPath    string
		wantSession bool
	}{
		{"installed game is matched", Game{ID: "g", Title: "G", Executable: exe, InstallDir: dir}, exe, true},
		{"uninstalled game is skipped", Game{ID: "g", Title: "G", Executable: exe, InstallDir: dir, Uninstalled: true}, exe, false},
		{"game without a location is skipped", Game{ID: "g", Title: "G"}, stranger, false},
		{"game without a location is not everything", Game{ID: "g", Title: "G"}, exe, false},
		{"directory-only game is matched by directory", Game{ID: "g", Title: "G", InstallDir: dir}, filepath.Join(dir, "bin", "run.exe"), true},
		{"executable-only game is matched by executable", Game{ID: "g", Title: "G", Executable: exe}, exe, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := seedGames(t, filepath.Join(t.TempDir(), "library.json"), []Game{tc.game})
			s.scan = func(context.Context) ([]procs.Process, bool, error) {
				return []procs.Process{{PID: 55, Path: tc.procPath, CreatedAt: time.Now()}}, true, nil
			}
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			s.AddSessionWatcher(watcher)

			s.detectTick(context.Background())

			s.mu.Lock()
			opened := len(s.running)
			s.mu.Unlock()
			if (opened == 1) != tc.wantSession {
				t.Fatalf("sessions opened = %d, want session = %v", opened, tc.wantSession)
			}
			select {
			case <-watcher.started:
				if !tc.wantSession {
					t.Fatal("SessionStarted fired for a game that must not match")
				}
			default:
				if tc.wantSession {
					t.Fatal("SessionStarted did not fire for a matching game")
				}
			}
		})
	}
}

func TestDetectLeavesAGameThatIsStillStartingToPlayGame(t *testing.T) {
	f := newDetectFixture(t)
	f.scanReturns([]procs.Process{{PID: 4002, Path: f.exe, CreatedAt: f.base}}, true)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	f.s.AddSessionWatcher(watcher)

	f.s.mu.Lock()
	f.s.starting = map[string]context.CancelFunc{f.game.ID: func() {}}
	f.s.mu.Unlock()

	f.s.detectTick(context.Background())

	f.s.mu.Lock()
	opened := len(f.s.running)
	f.s.mu.Unlock()
	if opened != 0 {
		t.Fatalf("detection opened %d session(s) for a game PlayGame is still starting, giving it two owners", opened)
	}
	select {
	case g := <-watcher.started:
		t.Fatalf("SessionStarted for a game that is still starting: %+v", g)
	default:
	}
}

func TestDetectAdoptsProcessStartTimeOfALaunchedSession(t *testing.T) {
	f := newDetectFixture(t)
	proc := newRegProcess(4003)
	f.s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	watcher := recordingWatcher{started: make(chan Game, 2), stopped: make(chan string, 1)}
	f.s.AddSessionWatcher(watcher)

	if err := f.s.PlayGame(f.game.ID); err != nil {
		t.Fatal(err)
	}
	if sess := f.session(t); sess == nil || !sess.createdAt.IsZero() || sess.external {
		t.Fatalf("setup: launched session = %+v, want one without a known process start time", sess)
	}

	created := f.base.Add(-2 * time.Second)
	f.clock.set(f.base.Add(5 * time.Second))
	f.scanReturns([]procs.Process{{PID: 4003, Path: f.exe, CreatedAt: created}}, true)
	f.s.detectTick(context.Background())

	sess := f.session(t)
	if sess == nil {
		t.Fatal("launched session disappeared while its process is still listed")
	}
	if !sess.createdAt.Equal(created) {
		t.Fatalf("createdAt = %v, want the process start time %v, which stop uses to verify the pid", sess.createdAt, created)
	}
	if !sess.lastSeen.Equal(f.base.Add(5 * time.Second)) {
		t.Fatalf("lastSeen = %v, want the heartbeat of this tick", sess.lastSeen)
	}
	if sess.external {
		t.Fatal("detection turned a launched session into an external one")
	}
	if len(watcher.started) != 1 {
		t.Fatalf("SessionStarted fired %d times, want once: detection must not announce a launched game again", len(watcher.started))
	}

	proc.exitNow()
	f.s.sessionWG.Wait()
}

func TestDetectUnknownProcessStartCountsFromDetection(t *testing.T) {
	f := newDetectFixture(t)
	f.clock.set(f.base.Add(10 * time.Second))
	f.scanReturns([]procs.Process{{PID: 4004, Path: f.exe, CreatedAtUnknown: true}}, true)

	f.s.detectTick(context.Background())

	sess := f.session(t)
	if sess == nil {
		t.Fatal("session not opened")
	}
	if !sess.startedAt.Equal(f.base.Add(10 * time.Second)) {
		t.Fatalf("startedAt = %v, want the moment of detection %v", sess.startedAt, f.base.Add(10*time.Second))
	}
}
