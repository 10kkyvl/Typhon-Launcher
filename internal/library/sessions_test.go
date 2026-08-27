package library

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/usagestats"
)

func TestPlayGameTracksSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Session Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(s.GetRunningGames()) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("session never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if s.GetInstalledGames()[0].LastPlayed == nil {
		t.Fatal("last played not set")
	}
	if mustServiceAt(t, path).GetInstalledGames()[0].LastPlayed == nil {
		t.Fatal("last played not persisted")
	}
}

func TestPlayGameRunsInExecutableDir(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "Game.v1.0")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(gameDir, "game.exe")
	data, err := os.ReadFile(`C:\Windows\System32\cmd.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, data, 0o755); err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.AddGame(exe, "Nested Game")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.InstallDir = root
	stored.LaunchArgs = []string{"/C", "cd > cwd.txt"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(s.GetRunningGames()) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("session never finished")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := os.Stat(filepath.Join(gameDir, "cwd.txt")); err != nil {
		t.Fatalf("game did not run in executable dir: %v", err)
	}
}

type recordingWatcher struct {
	started chan Game
	stopped chan string
}

func (r recordingWatcher) SessionStarted(game Game) { r.started <- game }

func (r recordingWatcher) SessionStopped(gameID string) { r.stopped <- gameID }

func TestSessionWatcherSeesStartAndStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Watcher Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case started := <-watcher.started:
		if started.ID != game.ID || started.Title != "Watcher Test" {
			t.Fatalf("started = %+v", started)
		}
	default:
		t.Fatal("watcher did not see the start")
	}

	select {
	case stopped := <-watcher.stopped:
		if stopped != game.ID {
			t.Fatalf("stopped = %q", stopped)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("watcher did not see the stop")
	}
}

type usageRecorder struct {
	mu     sync.Mutex
	events []usagestats.Event
}

func (r *usageRecorder) record(ev usagestats.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *usageRecorder) snapshot() []usagestats.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]usagestats.Event, len(r.events))
	copy(out, r.events)
	return out
}

func TestAddSessionWatcherMultipleWatchersNotified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	first := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	second := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(first)
	s.AddSessionWatcher(second)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Multi Watcher")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	for _, w := range []recordingWatcher{first, second} {
		select {
		case started := <-w.started:
			if started.ID != game.ID {
				t.Fatalf("started = %+v", started)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("watcher did not see the start")
		}
	}
	for _, w := range []recordingWatcher{first, second} {
		select {
		case stopped := <-w.stopped:
			if stopped != game.ID {
				t.Fatalf("stopped = %q", stopped)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("watcher did not see the stop")
		}
	}
}

func TestAddSessionWatcherNilIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	s.AddSessionWatcher(nil)
	s.mu.Lock()
	count := len(s.watchers)
	s.mu.Unlock()
	if count != 0 {
		t.Fatalf("watchers after nil = %d, want 0", count)
	}

	s.AddSessionWatcher(recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)})
	s.mu.Lock()
	count = len(s.watchers)
	s.mu.Unlock()
	if count != 1 {
		t.Fatalf("watchers after real watcher = %d, want 1", count)
	}
}

func TestPlayGameFailedStartSkipsSessionEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Broken")
	if err != nil {
		t.Fatal(err)
	}

	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)
	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)

	s.mu.Lock()
	s.findLocked(game.ID).Executable = filepath.Dir(exe)
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("expected error for failed start")
	}

	select {
	case started := <-watcher.started:
		t.Fatalf("unexpected SessionStarted: %+v", started)
	default:
	}
	select {
	case stopped := <-watcher.stopped:
		t.Fatalf("unexpected SessionStopped: %q", stopped)
	default:
	}
	if events := rec.snapshot(); len(events) != 0 {
		t.Fatalf("unexpected usage events: %+v", events)
	}
}

func TestFinishSessionRecordsUsageDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Duration Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).CanonicalGameID = "12345"
	s.mu.Unlock()

	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)

	startedAt := time.Now().Add(-7 * time.Second)
	s.finishSession(game.ID, startedAt)

	events := rec.snapshot()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	ev := events[0]
	if ev.Type != usagestats.TypeGameStopped {
		t.Fatalf("type = %q", ev.Type)
	}
	if ev.Properties.GameID != "12345" {
		t.Fatalf("game id = %q", ev.Properties.GameID)
	}
	if ev.Properties.DurationSeconds < 6 || ev.Properties.DurationSeconds > 9 {
		t.Fatalf("duration = %d", ev.Properties.DurationSeconds)
	}
	if ev.Timestamp.IsZero() {
		t.Fatal("timestamp not set")
	}
}

func TestFinishSessionEmptyGameIDWhenGameRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Removed Mid Session")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).CanonicalGameID = "999"
	s.mu.Unlock()

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}

	rec := &usageRecorder{}
	s.SetUsageRecorder(rec.record)

	s.finishSession(game.ID, time.Now().Add(-3*time.Second))

	events := rec.snapshot()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Type != usagestats.TypeGameStopped {
		t.Fatalf("type = %q", events[0].Type)
	}
	if events[0].Properties.GameID != "" {
		t.Fatalf("game id = %q, want empty", events[0].Properties.GameID)
	}
}

func TestAddSessionWatcherRaceWithSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	game, err := s.AddGame(`C:\Windows\System32\cmd.exe`, "Race Test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = []string{"/C", "exit"}
	s.mu.Unlock()

	tracker := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(tracker)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.AddSessionWatcher(recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)})
		}()
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case <-tracker.stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("session never finished")
	}

	wg.Wait()
}

func TestServiceShutdownWaitsForSessionCallbacks(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	// Written by the notify goroutine, read here after shutdown. Without the
	// WaitGroup the read races the write and outlives the service, which is
	// how a persist ends up running after shutdown.
	var got struct {
		gameID  string
		seconds int64
	}
	release := make(chan struct{})
	svc.SetOnSessionEnded(func(gameID string, seconds int64) {
		<-release
		got.gameID = gameID
		got.seconds = seconds
	})

	svc.finishSession("game-1", time.Now().Add(-5*time.Second))

	close(release)
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	if got.gameID != "game-1" {
		t.Fatalf("callback did not complete before shutdown returned: %+v", got)
	}
}
