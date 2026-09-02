package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/procs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func mustDir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustExe(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectOpensSessionForProcessInsideInstallDir(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Foo"})
	if err != nil {
		t.Fatal(err)
	}

	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	created := time.Now().Add(-5 * time.Minute)
	procPath := filepath.Join(dir, "bin", "launcher.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 111, Path: procPath, CreatedAt: created}}, nil
	}

	s.detectTick(context.Background())

	if !s.IsRunning(game.ID) {
		t.Fatal("session not opened for process inside InstallDir")
	}
	select {
	case started := <-watcher.started:
		if started.ID != game.ID {
			t.Fatalf("started = %+v, want id %s", started, game.ID)
		}
	default:
		t.Fatal("watcher did not observe SessionStarted")
	}

	s.mu.Lock()
	sess := s.running[game.ID]
	s.mu.Unlock()
	if sess == nil {
		t.Fatal("session missing from running map")
	}
	if !sess.startedAt.Equal(created) {
		t.Fatalf("startedAt = %v, want process CreatedAt %v", sess.startedAt, created)
	}
	if !sess.external {
		t.Fatal("session opened by detection should be marked external")
	}
}

func TestDetectComponentWisePathMismatch(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Foo"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Foo"})
	if err != nil {
		t.Fatal(err)
	}

	procPath := filepath.Join(root, "Foo2", "game.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 222, Path: procPath, CreatedAt: time.Now()}}, nil
	}

	s.detectTick(context.Background())

	if s.IsRunning(game.ID) {
		t.Fatal("process in C:\\...\\Foo2 matched InstallDir C:\\...\\Foo")
	}
}

func TestDetectCaseInsensitivePathMatch(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Case"})
	if err != nil {
		t.Fatal(err)
	}

	procPath := strings.ToUpper(filepath.Join(dir, "game.exe"))
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 333, Path: procPath, CreatedAt: time.Now()}}, nil
	}

	s.detectTick(context.Background())

	if !s.IsRunning(game.ID) {
		t.Fatal("case-insensitive path match did not open a session")
	}
}

func TestDetectLongestInstallDirWins(t *testing.T) {
	root := t.TempDir()
	outerDir := mustDir(t, root)
	outerExe := mustExe(t, filepath.Join(root, "outer.exe"))
	innerDir := mustDir(t, filepath.Join(root, "Inner"))
	innerExe := mustExe(t, filepath.Join(innerDir, "inner.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	outer, err := s.RegisterInstalled(InstalledGame{Executable: outerExe, InstallDir: outerDir, Title: "Outer"})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := s.RegisterInstalled(InstalledGame{Executable: innerExe, InstallDir: innerDir, Title: "Inner"})
	if err != nil {
		t.Fatal(err)
	}

	procPath := filepath.Join(innerDir, "game.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 444, Path: procPath, CreatedAt: time.Now()}}, nil
	}

	s.detectTick(context.Background())

	if !s.IsRunning(inner.ID) {
		t.Fatal("longest matching InstallDir (Inner) did not win")
	}
	if s.IsRunning(outer.ID) {
		t.Fatal("shorter InstallDir (root) incorrectly matched")
	}
}

func TestDetectPathUnknownProcessIgnored(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Unknown"})
	if err != nil {
		t.Fatal(err)
	}

	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 555, PathUnknown: true, CreatedAt: time.Now()}}, nil
	}

	s.detectTick(context.Background())

	if s.IsRunning(game.ID) {
		t.Fatal("process with PathUnknown opened a session by filename match")
	}
}

func TestDetectUninstallerExcluded(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Uninstaller"})
	if err != nil {
		t.Fatal(err)
	}

	procPath := filepath.Join(dir, "unins000.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 666, Path: procPath, CreatedAt: time.Now()}}, nil
	}

	s.detectTick(context.Background())

	if s.IsRunning(game.ID) {
		t.Fatal("unins000.exe inside InstallDir opened a session")
	}
}

func TestDetectScanErrorLeavesRunningUntouched(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	running, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Running"})
	if err != nil {
		t.Fatal(err)
	}
	otherDir := mustDir(t, filepath.Join(root, "Other"))
	otherExe := mustExe(t, filepath.Join(otherDir, "other.exe"))
	notRunning, err := s.RegisterInstalled(InstalledGame{Executable: otherExe, InstallDir: otherDir, Title: "NotRunning"})
	if err != nil {
		t.Fatal(err)
	}

	procPath := filepath.Join(dir, "game.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 777, Path: procPath, CreatedAt: time.Now()}}, nil
	}
	s.detectTick(context.Background())
	if !s.IsRunning(running.ID) {
		t.Fatal("setup: session was not opened before the error scan")
	}

	scanErr := errors.New("enumerate failed")
	s.scan = func(context.Context) ([]procs.Process, error) { return nil, scanErr }

	s.detectTick(context.Background())

	if !s.IsRunning(running.ID) {
		t.Fatal("scan error closed an already-open session")
	}
	if s.IsRunning(notRunning.ID) {
		t.Fatal("scan error opened a session that was never observed")
	}
}

func TestDetectSessionClosesAfterGracePeriodNotBefore(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Grace"})
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := base
	s.now = func() time.Time { return clock }
	s.watchInterval = time.Second

	procPath := filepath.Join(dir, "game.exe")
	created := base.Add(-time.Minute)
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 888, Path: procPath, CreatedAt: created}}, nil
	}
	s.detectTick(context.Background())
	if !s.IsRunning(game.ID) {
		t.Fatal("setup: session did not open")
	}

	s.scan = func(context.Context) ([]procs.Process, error) { return nil, nil }

	clock = base.Add(1500 * time.Millisecond)
	s.detectTick(context.Background())
	if !s.IsRunning(game.ID) {
		t.Fatal("session closed before 2*watchInterval elapsed since last seen")
	}

	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	clock = base.Add(2500 * time.Millisecond)
	s.detectTick(context.Background())
	if s.IsRunning(game.ID) {
		t.Fatal("session still running past the 2*watchInterval grace period")
	}
	select {
	case stopped := <-watcher.stopped:
		if stopped != game.ID {
			t.Fatalf("stopped id = %q, want %q", stopped, game.ID)
		}
	default:
		t.Fatal("watcher did not observe SessionStopped")
	}

	reloaded, err := NewServiceAt(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	games := reloaded.GetInstalledGames()
	if len(games) != 1 || games[0].PlaytimeSeconds <= 0 {
		t.Fatalf("detected session close did not persist playtime: %+v", games)
	}
	if err := reloaded.ServiceShutdown(); err != nil {
		t.Fatalf("reloaded ServiceShutdown: %v", err)
	}
}

func TestServiceStartupRunsImmediateScanBeforeFirstTick(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Immediate"})
	if err != nil {
		t.Fatal(err)
	}

	s.watchInterval = time.Hour
	procPath := filepath.Join(dir, "game.exe")
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 999, Path: procPath, CreatedAt: time.Now()}}, nil
	}
	// SessionStarted fires only after detectTick has already inserted the
	// session into s.running, so waiting on it (rather than on scan being
	// merely invoked) is race-free synchronization with the loop goroutine.
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	if !procs.Supported() {
		t.Skip("procs enumeration unsupported on this platform, ServiceStartup does not start the loop")
	}

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	select {
	case started := <-watcher.started:
		if started.ID != game.ID {
			t.Fatalf("started = %+v, want id %s", started, game.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceStartup did not run an immediate scan before the first tick")
	}
	if !s.IsRunning(game.ID) {
		t.Fatal("immediate scan on startup did not open the already-running session")
	}
}

func TestDetectKeepsLaunchedSessionAliveWhilePathUnverifiable(t *testing.T) {
	root := t.TempDir()
	s := mustServiceAt(t, filepath.Join(root, "library.json"))

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := base
	s.now = func() time.Time { return clock }
	s.watchInterval = time.Second

	exe, exitArgs := testExecutable(t)
	game, err := s.AddGame(exe, "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = exitArgs
	// Simulate an active detector without running the real loop: this is
	// what makes the PlayGame goroutine defer closing to detectTick instead
	// of closing the session itself the instant the child process exits.
	s.watching = true
	s.mu.Unlock()

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()

	if !s.IsRunning(game.ID) {
		t.Fatal("session launched by PlayGame closed itself despite an active detector")
	}

	s.mu.Lock()
	pid := s.running[game.ID].pid
	s.mu.Unlock()
	if pid == 0 {
		t.Fatal("setup: launched session has no recorded pid")
	}

	// The OS can enumerate the pid but not its path or start time: this is
	// exactly what happens for an anti-cheat protected or elevated process
	// when the launcher itself is not elevated (procs_windows.go
	// inspectProcess sets both PathUnknown and CreatedAtUnknown when
	// OpenProcess fails). Path-based matching can never succeed for this
	// process, yet the game genuinely never stopped.
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: pid, PathUnknown: true, CreatedAtUnknown: true}}, nil
	}

	for i := 0; i < 5; i++ {
		clock = clock.Add(3 * time.Second) // well past 2*watchInterval each tick
		s.detectTick(context.Background())
		if !s.IsRunning(game.ID) {
			t.Fatalf("session closed on tick %d despite its pid still being present in the process table", i)
		}
	}

	// Once the OS genuinely stops reporting the pid, the session must still
	// close via the grace-period path.
	s.scan = func(context.Context) ([]procs.Process, error) { return nil, nil }
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	clock = clock.Add(3 * time.Second)
	s.detectTick(context.Background())
	if s.IsRunning(game.ID) {
		t.Fatal("session stayed running after its pid genuinely disappeared and the grace period elapsed")
	}
	select {
	case stopped := <-watcher.stopped:
		if stopped != game.ID {
			t.Fatalf("stopped id = %q, want %q", stopped, game.ID)
		}
	default:
		t.Fatal("watcher did not observe SessionStopped once the pid disappeared")
	}
}

func TestDetectRejectsHeartbeatWhenCreatedAtMismatches(t *testing.T) {
	root := t.TempDir()
	dir := mustDir(t, filepath.Join(root, "Game"))
	exe := mustExe(t, filepath.Join(dir, "game.exe"))

	s := mustServiceAt(t, filepath.Join(root, "library.json"))
	game, err := s.RegisterInstalled(InstalledGame{Executable: exe, InstallDir: dir, Title: "Reused"})
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := base
	s.now = func() time.Time { return clock }
	s.watchInterval = time.Second

	procPath := filepath.Join(dir, "game.exe")
	created := base.Add(-time.Minute)
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 1010, Path: procPath, CreatedAt: created}}, nil
	}
	s.detectTick(context.Background())
	if !s.IsRunning(game.ID) {
		t.Fatal("setup: session did not open")
	}

	// Same pid reappears with an unmatchable path (so it cannot heartbeat via
	// matches) and a different CreatedAt: the OS reused the pid for an
	// unrelated process, so the recorded session must not be kept alive by
	// raw pid presence alone.
	s.scan = func(context.Context) ([]procs.Process, error) {
		return []procs.Process{{PID: 1010, PathUnknown: true, CreatedAt: created.Add(time.Hour)}}, nil
	}
	clock = clock.Add(1500 * time.Millisecond)
	s.detectTick(context.Background())
	if !s.IsRunning(game.ID) {
		t.Fatal("session closed before the grace period elapsed")
	}

	clock = clock.Add(2 * time.Second)
	s.detectTick(context.Background())
	if s.IsRunning(game.ID) {
		t.Fatal("session kept alive by a pid whose recorded CreatedAt no longer matches (recycled pid)")
	}
}

func TestDetectRaceWithPlayGameAndGetRunningGames(t *testing.T) {
	root := t.TempDir()
	s := mustServiceAt(t, filepath.Join(root, "library.json"))

	s.scan = func(context.Context) ([]procs.Process, error) { return nil, nil }
	s.watchInterval = 10 * time.Millisecond

	exe, exitArgs := testExecutable(t)
	game, err := s.AddGame(exe, "Race")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).LaunchArgs = exitArgs
	s.mu.Unlock()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.detectTick(context.Background())
			}
		}
	}()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.GetRunningGames()
		}()
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()

	close(stop)
	wg.Wait()
}
