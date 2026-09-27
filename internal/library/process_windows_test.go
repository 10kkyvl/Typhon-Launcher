//go:build windows && !devmock

package library

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"

	"typhon/internal/uierr"
)

const elevatedExe = `C:\Windows\regedit.exe`

// requireElevationPrompt skips unless CreateProcess of elevatedExe fails with
// ERROR_ELEVATION_REQUIRED here, the same way it does for a game with
// requireAdministrator in its manifest.
func requireElevationPrompt(t *testing.T) {
	t.Helper()
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("тест идёт с правами администратора: ERROR_ELEVATION_REQUIRED не воспроизводится")
	}
	if _, err := os.Stat(elevatedExe); err != nil {
		t.Skipf("нет %s: %v", elevatedExe, err)
	}
	probe := exec.Command(elevatedExe)
	err := probe.Start()
	if err == nil {
		if killErr := probe.Process.Kill(); killErr != nil {
			t.Fatalf("kill probe: %v", killErr)
		}
		var exit *exec.ExitError
		if waitErr := probe.Wait(); waitErr != nil && !errors.As(waitErr, &exit) {
			t.Fatalf("wait probe: %v", waitErr)
		}
		t.Skipf("%s запустился без повышения: пользователь не администратор", elevatedExe)
	}
	if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		t.Fatalf("probe start error = %v, want ERROR_ELEVATION_REQUIRED", err)
	}
}

// longProcess starts a real child and wraps a handle to it the way
// startElevated wraps the one ShellExecuteEx returns.
func longProcess(t *testing.T) *handleProcess {
	t.Helper()
	cmd := exec.Command(`C:\Windows\System32\cmd.exe`, "/C", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Тест обычно уже убил процесс через handleProcess, а TerminateProcess
		// на завершённом процессе отвечает ERROR_ACCESS_DENIED.
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Errorf("kill child: %v", err)
		}
		var exit *exec.ExitError
		if err := cmd.Wait(); err != nil && !errors.As(err, &exit) {
			t.Errorf("wait child: %v", err)
		}
	})
	//nolint:gosec // G115: PID из os/exec укладывается в uint32 на Windows
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	return &handleProcess{handle: handle, id: cmd.Process.Pid}
}

func TestElevatingStarterFallsBackOnElevationRequired(t *testing.T) {
	requireElevationPrompt(t)
	want := &handleProcess{id: 4242}
	var got launch
	start := elevatingStarter(execStarter, func(req launch) (gameProcess, error) {
		got = req
		return want, nil
	})
	req := launch{executable: elevatedExe, args: []string{"/s", `C:\Program Files\x.reg`}, workDir: `C:\Windows`}

	proc, err := start(t.Context(), req)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if proc != want {
		t.Fatalf("start returned %v, want the elevated process", proc)
	}
	if got.executable != req.executable || got.workDir != req.workDir || len(got.args) != 2 || got.args[1] != req.args[1] {
		t.Fatalf("elevate got %+v, want %+v", got, req)
	}
}

func TestElevatingStarterKeepsOtherErrors(t *testing.T) {
	var calls atomic.Int32
	start := elevatingStarter(execStarter, func(launch) (gameProcess, error) {
		calls.Add(1)
		return nil, errors.New("elevate must not be called")
	})
	missing := filepath.Join(t.TempDir(), "game.exe")

	_, err := start(t.Context(), launch{executable: missing, workDir: filepath.Dir(missing)})
	if err == nil {
		t.Fatal("start of a missing executable succeeded")
	}
	if calls.Load() != 0 {
		t.Fatalf("a plain start failure went to UAC (%d calls): %v", calls.Load(), err)
	}
}

func TestPlayGameElevationDeclinedKeepsItsCode(t *testing.T) {
	requireElevationPrompt(t)
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).Executable = elevatedExe
	s.mu.Unlock()
	var calls atomic.Int32
	s.start = elevatingStarter(execStarter, func(launch) (gameProcess, error) {
		calls.Add(1)
		return nil, errElevationDeclined
	})

	for attempt := 1; attempt <= 2; attempt++ {
		err := s.PlayGame(game.ID)
		if code := uierr.Code(err); code != "library.elevation_declined" {
			t.Fatalf("attempt %d: PlayGame error code = %q (%v), want library.elevation_declined", attempt, code, err)
		}
		if int(calls.Load()) != attempt {
			t.Fatalf("attempt %d: elevate called %d times: a declined prompt left the game stuck as starting", attempt, calls.Load())
		}
	}
	if s.IsRunning(game.ID) {
		t.Fatal("declined launch opened a session")
	}
}

func TestPlayGameTracksElevatedProcess(t *testing.T) {
	requireElevationPrompt(t)
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).Executable = elevatedExe
	s.mu.Unlock()
	proc := longProcess(t)
	s.start = elevatingStarter(execStarter, func(launch) (gameProcess, error) { return proc, nil })

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame: %v", err)
	}
	if !s.IsRunning(game.ID) {
		t.Fatal("elevated game has no session")
	}
	if err := s.StopGame(game.ID); err != nil {
		t.Fatalf("StopGame: %v", err)
	}
	s.sessionWG.Wait()
	if s.IsRunning(game.ID) {
		t.Fatal("session survived the elevated process exit")
	}
}

func TestHandleProcessKillAfterExit(t *testing.T) {
	proc := longProcess(t)
	waited := make(chan error, 1)
	go func() { waited <- proc.wait() }()

	if err := proc.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := <-waited; err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := proc.kill(); !errors.Is(err, errSessionProcessGone) {
		t.Fatalf("kill after exit = %v, want errSessionProcessGone", err)
	}
}

func TestStartElevatedRejectsBadPath(t *testing.T) {
	_, err := startElevated(launch{executable: "C:" + string(rune(0)) + `\game.exe`})
	if err == nil {
		t.Fatal("startElevated с нулевым байтом в пути вернул успех")
	}
	if errors.Is(err, errElevationDeclined) {
		t.Fatalf("bad path reported as a declined prompt: %v", err)
	}
}
