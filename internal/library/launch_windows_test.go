package library

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const elevatedExe = `C:\Windows\regedit.exe`

type fakeGameProcess struct {
	kills atomic.Int32
}

func (p *fakeGameProcess) Kill() error {
	p.kills.Add(1)
	return nil
}

type elevateCall struct {
	exe  string
	args []string
	dir  string
}

// elevationGame returns a library entry whose executable makes CreateProcess
// fail with ERROR_ELEVATION_REQUIRED on this machine, the same way a game
// with requireAdministrator in its manifest does.
func elevationGame(t *testing.T, s *Service) Game {
	t.Helper()
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("тест идёт с правами администратора: ERROR_ELEVATION_REQUIRED не воспроизводится")
	}
	if _, err := os.Stat(elevatedExe); err != nil {
		t.Skipf("нет %s: %v", elevatedExe, err)
	}
	probe := exec.Command(elevatedExe)
	if err := probe.Start(); err == nil {
		if killErr := probe.Process.Kill(); killErr != nil {
			t.Fatalf("kill probe: %v", killErr)
		}
		var exit *exec.ExitError
		if waitErr := probe.Wait(); waitErr != nil && !errors.As(waitErr, &exit) {
			t.Fatalf("wait probe: %v", waitErr)
		}
		t.Skipf("%s запустился без повышения: пользователь не администратор", elevatedExe)
	} else if !needsElevation(err) {
		t.Fatalf("probe start error = %v, want ERROR_ELEVATION_REQUIRED", err)
	}

	game, err := s.AddGame(tempGameExe(t), "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.Executable = elevatedExe
	stored.LaunchArgs = []string{"/s", `C:\Program Files\x.reg`}
	game = *stored
	s.mu.Unlock()
	return game
}

func TestPlayGameFallsBackToElevation(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game := elevationGame(t, s)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	proc := &fakeGameProcess{}
	release := make(chan struct{})
	calls := make(chan elevateCall, 1)
	s.elevate = func(exe string, args []string, dir string) (launched, error) {
		calls <- elevateCall{exe: exe, args: args, dir: dir}
		return launched{process: proc, pid: 4242, wait: func() error { <-release; return nil }}, nil
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame: %v", err)
	}
	call := <-calls
	if call.exe != elevatedExe || call.dir != `C:\Windows` {
		t.Fatalf("elevate(%q, dir %q), want %q in C:\\Windows", call.exe, call.dir, elevatedExe)
	}
	if len(call.args) != 2 || call.args[1] != `C:\Program Files\x.reg` {
		t.Fatalf("elevate args = %q, launch args lost", call.args)
	}
	if !s.IsRunning(game.ID) {
		t.Fatal("elevated game has no session")
	}
	select {
	case got := <-watcher.started:
		if got.ID != game.ID {
			t.Fatalf("SessionStarted(%q), want %q", got.ID, game.ID)
		}
	default:
		t.Fatal("SessionStarted not sent for elevated game")
	}

	if err := s.StopGame(game.ID); err != nil {
		t.Fatalf("StopGame: %v", err)
	}
	if proc.kills.Load() != 1 {
		t.Fatalf("elevated process killed %d times, want 1", proc.kills.Load())
	}

	close(release)
	s.sessionWG.Wait()
	if s.IsRunning(game.ID) {
		t.Fatal("session survived the elevated process exit")
	}
	if got := <-watcher.stopped; got != game.ID {
		t.Fatalf("SessionStopped(%q), want %q", got, game.ID)
	}
}

func TestPlayGameElevationDeclined(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game := elevationGame(t, s)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	var calls atomic.Int32
	s.elevate = func(string, []string, string) (launched, error) {
		calls.Add(1)
		return launched{}, errElevationDeclined
	}

	for attempt := 1; attempt <= 2; attempt++ {
		err := s.PlayGame(game.ID)
		if !errors.Is(err, errElevationDeclined) {
			t.Fatalf("attempt %d: PlayGame error = %v, want errElevationDeclined", attempt, err)
		}
		if int(calls.Load()) != attempt {
			t.Fatalf("attempt %d: elevate called %d times: declined launch left the game stuck as launching", attempt, calls.Load())
		}
	}
	if s.IsRunning(game.ID) {
		t.Fatal("declined launch opened a session")
	}
	select {
	case got := <-watcher.started:
		t.Fatalf("SessionStarted(%q) after declined launch", got.ID)
	default:
	}
}

func TestPlayGameReleasesLockWhileElevating(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game := elevationGame(t, s)

	entered := make(chan struct{})
	answer := make(chan struct{})
	s.elevate = func(string, []string, string) (launched, error) {
		close(entered)
		<-answer
		return launched{}, errElevationDeclined
	}

	played := make(chan error, 1)
	go func() { played <- s.PlayGame(game.ID) }()
	select {
	case <-entered:
	case err := <-played:
		t.Fatalf("PlayGame returned %v without asking for elevation", err)
	}

	probed := make(chan error, 1)
	go func() {
		s.GetRunningGames()
		probed <- s.PlayGame(game.ID)
	}()
	select {
	case err := <-probed:
		if !errors.Is(err, errSessionLaunching) {
			t.Fatalf("second PlayGame during UAC = %v, want errSessionLaunching", err)
		}
	case <-time.After(10 * time.Second):
		close(answer)
		t.Fatal("library mutex held while waiting for the UAC answer")
	}

	close(answer)
	if err := <-played; !errors.Is(err, errElevationDeclined) {
		t.Fatalf("PlayGame = %v, want errElevationDeclined", err)
	}
}

func TestPlayGameElevationAdoptsDetectedSession(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game := elevationGame(t, s)
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)

	proc := &fakeGameProcess{}
	release := make(chan struct{})
	s.elevate = func(string, []string, string) (launched, error) {
		fakeExternalSession(s, game.ID, 4242, time.Now())
		return launched{process: proc, pid: 4242, wait: func() error { <-release; return nil }}, nil
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame: %v", err)
	}
	select {
	case got := <-watcher.started:
		t.Fatalf("SessionStarted(%q) sent twice for a session the detector already opened", got.ID)
	default:
	}
	if err := s.StopGame(game.ID); err != nil {
		t.Fatalf("StopGame: %v", err)
	}
	if proc.kills.Load() != 1 {
		t.Fatalf("adopted process killed %d times, want 1", proc.kills.Load())
	}
	close(release)
	s.sessionWG.Wait()
}

func TestPlayGameElevatedGameRemovedDuringPrompt(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game := elevationGame(t, s)

	release := make(chan struct{})
	waited := make(chan struct{})
	s.elevate = func(string, []string, string) (launched, error) {
		s.mu.Lock()
		s.games = s.games[:0]
		s.mu.Unlock()
		return launched{process: &fakeGameProcess{}, pid: 4242, wait: func() error {
			<-release
			close(waited)
			return nil
		}}, nil
	}

	if err := s.PlayGame(game.ID); !errors.Is(err, errSessionGameRemoved) {
		t.Fatalf("PlayGame = %v, want errSessionGameRemoved", err)
	}
	if s.IsRunning(game.ID) {
		t.Fatal("session opened for a game no longer in the library")
	}
	close(release)
	s.sessionWG.Wait()
	select {
	case <-waited:
	default:
		t.Fatal("process handle of the orphaned launch was never waited on and released")
	}
}

func TestHandleProcessWaitAndKill(t *testing.T) {
	cmd := exec.Command(`C:\Windows\System32\cmd.exe`, "/C", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G115: PID из os/exec укладывается в uint32 на Windows
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	proc := &handleProcess{handle: handle}
	waited := make(chan error, 1)
	go func() { waited <- proc.wait() }()

	if err := proc.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if err := <-waited; err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := proc.Kill(); !errors.Is(err, errSessionProcessGone) {
		t.Fatalf("Kill after exit = %v, want errSessionProcessGone", err)
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); err != nil && !errors.As(err, &exit) {
		t.Fatalf("cmd.Wait: %v", err)
	}
}
