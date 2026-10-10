package selfupdate

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readyService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("installer"))
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.0.0", readyPath: readyPath}
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return s
}

func restartBudget(t *testing.T, wait, grace time.Duration) {
	t.Helper()
	prevWait, prevGrace := parentExitTimeout, restartGrace
	parentExitTimeout, restartGrace = wait, grace
	t.Cleanup(func() { parentExitTimeout, restartGrace = prevWait, prevGrace })
}

// The worker waits parentExitTimeout for the launcher to exit and then gives up
// without relaunching. A launcher that is still alive after that has to say so:
// left in the applying state it refuses every click with ErrBusy for good.
func TestApplyUpdateReportsALauncherThatDoesNotExit(t *testing.T) {
	restartBudget(t, time.Millisecond, time.Millisecond)
	started := captureWorkerStarts(t)
	s := readyService(t)
	missed := make(chan struct{}, 2)
	s.onRestartMissed = func() { missed <- struct{}{} }

	if err := s.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate() error = %v", err)
	}

	select {
	case <-missed:
	case <-time.After(5 * time.Second):
		t.Fatalf("launcher still %v after the worker gave up: the user is stuck on the applying screen", s.GetStatus().State)
	}

	got := s.GetStatus()
	if got.State != StateReady || got.AvailableVersion != "1.2.3" {
		t.Fatalf("status = %+v, want the downloaded update ready for another try", got)
	}
	if got.ErrorCode != "apply" {
		t.Fatalf("ErrorCode = %q, want apply", got.ErrorCode)
	}
	if want := "typhon:selfupdate.parent_still_running:"; !strings.HasPrefix(got.Error, want) {
		t.Fatalf("Error = %q, want the %s code the UI translates", got.Error, want)
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		t.Fatal("service stayed busy: the retry would bounce off ErrBusy")
	}

	if err := s.ApplyUpdate("en"); err != nil {
		t.Fatalf("retry ApplyUpdate() error = %v", err)
	}
	if len(started.specs) != 2 {
		t.Fatalf("worker started %d times, want a second start for the retry", len(started.specs))
	}
}

// Once the process is on its way out nothing is left to tell: the watch must
// not outlive ServiceShutdown, or the exiting launcher repaints a screen that
// is already gone.
func TestServiceShutdownEndsTheRestartWatch(t *testing.T) {
	restartBudget(t, time.Hour, time.Hour)
	captureWorkerStarts(t)
	s := readyService(t)
	missed := make(chan struct{}, 1)
	s.onRestartMissed = func() { missed <- struct{}{} }

	if err := s.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.ServiceShutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServiceShutdown() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceShutdown() is stuck waiting for the restart watch")
	}
	select {
	case <-missed:
		t.Fatal("restart watch fired after shutdown")
	default:
	}
	if got := s.GetStatus().State; got != StateApplying {
		t.Fatalf("State = %v, want applying: the launcher is leaving", got)
	}
}
