//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "spec.json")
	want := updateSpec{InstallerPath: `C:\cache\setup.exe`, ParentPID: 4242, RelaunchPath: `C:\Program Files\Typhon\typhon.exe`}

	if err := writeUpdateSpec(path, want); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}
	got, err := readUpdateSpec(path)
	if err != nil {
		t.Fatalf("readUpdateSpec: %v", err)
	}
	if got != want {
		t.Fatalf("readUpdateSpec = %+v, want %+v", got, want)
	}
}

func TestReadUpdateSpecMissingFile(t *testing.T) {
	if _, err := readUpdateSpec(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("readUpdateSpec on a missing file returned nil error")
	}
}

func TestReadUpdateSpecCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write raw spec: %v", err)
	}
	if _, err := readUpdateSpec(path); err == nil {
		t.Fatal("readUpdateSpec on corrupt json returned nil error")
	}
}

func TestWorkerProcessAliveForSelf(t *testing.T) {
	alive, err := workerProcessAlive(os.Getpid())
	if err != nil {
		t.Fatalf("workerProcessAlive(self): %v", err)
	}
	if !alive {
		t.Fatal("workerProcessAlive(self) = false, want true")
	}
}

func TestWorkerProcessAliveZeroPID(t *testing.T) {
	alive, err := workerProcessAlive(0)
	if err != nil {
		t.Fatalf("workerProcessAlive(0): %v", err)
	}
	if alive {
		t.Fatal("workerProcessAlive(0) = true, want false")
	}
}

// cmdExePath returns the path to the system cmd.exe, built from %SystemRoot%
// rather than user input, for use as a short-lived stand-in process in tests
// (invariant 33 — this is not the kind of variable path that rule targets).
func cmdExePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	//nolint:gosec // G703: path is built from %SystemRoot%, a fixed system directory, not user input
	if _, err := os.Stat(path); err != nil {
		t.Skipf("cmd.exe unavailable: %v", err)
	}
	return path
}

func TestWorkerProcessAliveExitedProcess(t *testing.T) {
	cmdExe := cmdExePath(t)
	//nolint:gosec // G204: cmd.exe is a fixed system binary used as a short-lived stand-in process for this test
	cmd := exec.Command(cmdExe, "/C", "exit", "0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait stand-in process: %v", err)
	}

	alive, err := workerProcessAlive(pid)
	if err != nil {
		t.Fatalf("workerProcessAlive(exited): %v", err)
	}
	if alive {
		t.Fatal("workerProcessAlive() = true for a process that already exited")
	}
}

func TestWaitForParentExitSucceedsOnceParentIsGone(t *testing.T) {
	restore := parentPollInterval
	parentPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { parentPollInterval = restore })

	cmdExe := cmdExePath(t)
	//nolint:gosec // G204: cmd.exe is a fixed system binary used as a short-lived stand-in process for this test
	cmd := exec.Command(cmdExe, "/C", "exit", "0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait stand-in process: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitForParentExit(ctx, pid); err != nil {
		t.Fatalf("waitForParentExit() error = %v, want nil once the parent has exited", err)
	}
}

func TestWaitForParentExitTimesOutWhileParentStaysAlive(t *testing.T) {
	restore := parentPollInterval
	parentPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { parentPollInterval = restore })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// os.Getpid() (this test process) never exits during the test, so the wait
	// must hit the deadline rather than declare success.
	if err := waitForParentExit(ctx, os.Getpid()); !errors.Is(err, errParentStillRunning) {
		t.Fatalf("waitForParentExit() error = %v, want errParentStillRunning", err)
	}
}

// TestApplyContextBudgetSurvivesParentWaitTimeout closes КРИТ-2: before the
// fix, RunWorker built one context with parentExitTimeout and spent it on
// both waitForParentExit and Apply, so a parent-exit wait that ran to its own
// deadline left Apply/exec.CommandContext with a context that was already
// expired — killing the installer mid-write with no recovery path. This
// drives waitForParentExit to its own deadline against a pid that never
// exits (this test process), then checks that the independently-budgeted
// applyCtx still has time left.
func TestApplyContextBudgetSurvivesParentWaitTimeout(t *testing.T) {
	restoreWait, restoreApply, restorePoll := parentExitTimeout, applyTimeout, parentPollInterval
	parentExitTimeout = 20 * time.Millisecond
	applyTimeout = 2 * time.Second
	parentPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { parentExitTimeout, applyTimeout, parentPollInterval = restoreWait, restoreApply, restorePoll })

	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	if err := waitForParentExit(waitCtx, os.Getpid()); !errors.Is(err, errParentStillRunning) {
		t.Fatalf("waitForParentExit() error = %v, want errParentStillRunning", err)
	}

	if applyCtx.Err() != nil {
		t.Fatalf("applyCtx.Err() = %v, want nil: the installer's timeout budget must not be consumed by waiting for the parent to exit", applyCtx.Err())
	}
}

func TestNewWorkerContextsIndependentDeadlines(t *testing.T) {
	restoreWait, restoreApply := parentExitTimeout, applyTimeout
	parentExitTimeout = 20 * time.Millisecond
	applyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { parentExitTimeout, applyTimeout = restoreWait, restoreApply })

	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	<-waitCtx.Done()
	if !errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("waitCtx.Err() = %v, want context.DeadlineExceeded", waitCtx.Err())
	}
	if applyCtx.Err() != nil {
		t.Fatalf("applyCtx.Err() = %v, want nil: applyCtx has its own longer deadline", applyCtx.Err())
	}
}

func TestRelaunchEmptyPath(t *testing.T) {
	if err := relaunch(""); err == nil {
		t.Fatal("relaunch(\"\") returned nil error")
	}
}

func TestRelaunchMissingFile(t *testing.T) {
	if err := relaunch(filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("relaunch() on a missing file returned nil error")
	}
}

func TestStartUpdateWorkerMissingExecutable(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.json")
	if err := startUpdateWorker(filepath.Join(dir, "missing-launcher.exe"), specPath); err == nil {
		t.Fatal("startUpdateWorker() with a missing executable returned nil error")
	}
}

func TestDetachedProcAttrFlags(t *testing.T) {
	attr := detachedProcAttr()
	if !attr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if attr.CreationFlags == 0 {
		t.Fatal("CreationFlags = 0, want DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP set")
	}
}

func TestRunWorkerReturnsErrorWhenSpecUnreadable(t *testing.T) {
	if err := RunWorker(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("RunWorker on a missing spec file returned nil error")
	}
}

// TestRunWorkerRemovesSpecFileEvenOnFailure closes the invariant that the
// spec file is single-use: once RunWorker has read it (successfully or not
// past that point), a stale spec must not be picked up again by a future run.
func TestRunWorkerRemovesSpecFileEvenOnFailure(t *testing.T) {
	restorePoll, restoreTimeout := parentPollInterval, parentExitTimeout
	parentPollInterval = 5 * time.Millisecond
	parentExitTimeout = 100 * time.Millisecond
	t.Cleanup(func() { parentPollInterval, parentExitTimeout = restorePoll, restoreTimeout })

	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.json")
	spec := updateSpec{InstallerPath: filepath.Join(dir, "missing-setup.exe"), ParentPID: os.Getpid(), RelaunchPath: filepath.Join(dir, "missing-launcher.exe")}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}

	if err := RunWorker(specPath); err == nil {
		t.Fatal("RunWorker() error = nil, want an error: this test process (the parent) never exits")
	}

	if _, err := os.Stat(specPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spec file survived RunWorker: %v", err)
	}
}
