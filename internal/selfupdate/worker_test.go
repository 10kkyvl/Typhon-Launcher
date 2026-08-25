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
	t.Setenv("APPDATA", dir)
	specPath := filepath.Join(dir, "spec.json")
	spec := updateSpec{InstallerPath: filepath.Join(dir, "missing-setup.exe"), ParentPID: os.Getpid(), RelaunchPath: filepath.Join(dir, "missing-launcher.exe")}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}

	if err := runWorker(specPath, quietReporter); err == nil {
		t.Fatal("runWorker() error = nil, want an error: this test process (the parent) never exits")
	}

	if _, err := os.Stat(specPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spec file survived RunWorker: %v", err)
	}
}

func quietReporter(string, string) stageReporter { return silentReporter{} }

// TestEnsureReplacedDetectsUntouchedLauncher closes the bug that made the
// whole feature look like a no-op: the worker used to run from the very
// binary the NSIS installer has to overwrite, Windows kept that image locked,
// the installer skipped the file and still exited 0, and the launcher came
// back on the version it started from with nothing reported anywhere.
func TestEnsureReplacedDetectsUntouchedLauncher(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
		want   error
	}{
		{"installer left the binary byte-identical", "aa", "aa", ErrNotReplaced},
		{"binary disappeared", "aa", "", ErrNotReplaced},
		{"binary was never there and still is not", "", "", ErrNotReplaced},
		{"binary was replaced", "aa", "bb", nil},
		{"fresh install where nothing existed before", "", "bb", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureReplaced(tc.before, tc.after, `C:\Typhon\typhon.exe`)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ensureReplaced() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFileDigest(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	missing, err := fileDigest(ctx, filepath.Join(dir, "absent.exe"))
	if err != nil {
		t.Fatalf("fileDigest on a missing file returned an error: %v", err)
	}
	if missing != "" {
		t.Fatalf("fileDigest on a missing file = %q, want an empty digest", missing)
	}

	first := filepath.Join(dir, "first.exe")
	writeTestFile(t, first, []byte("old build"))
	second := filepath.Join(dir, "second.exe")
	writeTestFile(t, second, []byte("new build"))

	a, err := fileDigest(ctx, first)
	if err != nil {
		t.Fatalf("fileDigest(first): %v", err)
	}
	b, err := fileDigest(ctx, second)
	if err != nil {
		t.Fatalf("fileDigest(second): %v", err)
	}
	if a == "" || a == b {
		t.Fatalf("fileDigest gave %q and %q, want two different non-empty digests", a, b)
	}

	again, err := fileDigest(ctx, first)
	if err != nil {
		t.Fatalf("fileDigest(first) again: %v", err)
	}
	if again != a {
		t.Fatalf("fileDigest is not stable: %q then %q", a, again)
	}
}

func TestFileDigestHonoursCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.exe")
	writeTestFile(t, path, []byte("payload"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fileDigest(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("fileDigest() error = %v, want context.Canceled", err)
	}
}

func TestCopyExecutableProducesAnIndependentCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, src, []byte("launcher bytes"))
	dst := filepath.Join(dir, "worker", "typhon-update.exe")

	if err := copyExecutable(src, dst); err != nil {
		t.Fatalf("copyExecutable: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(got) != "launcher bytes" {
		t.Fatalf("copy content = %q, want the source bytes", got)
	}
	if err := os.Remove(src); err != nil {
		t.Fatalf("the copy still depends on the source: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("copy vanished with the source: %v", err)
	}
}

func TestCopyExecutableMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := copyExecutable(filepath.Join(dir, "absent.exe"), filepath.Join(dir, "worker", "copy.exe")); err == nil {
		t.Fatal("copyExecutable() error = nil, want an error for a missing source")
	}
}

// TestRunWorkerRecordsFailureForTheRelaunchedLauncher closes the other half of
// the same bug: the install happens with no UI on screen, so a failure that is
// not written down looks to the user like a launcher that closed and came back
// for no reason.
func TestRunWorkerRecordsFailureForTheRelaunchedLauncher(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	appDir := filepath.Join(configDir, "Typhon")

	specPath := filepath.Join(dir, "spec.json")
	spec := updateSpec{
		InstallerPath: filepath.Join(dir, "missing-setup.exe"),
		ParentPID:     0,
		RelaunchPath:  filepath.Join(dir, "missing-launcher.exe"),
		Version:       "1.2.3",
	}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}

	if err := runWorker(specPath, quietReporter); err == nil {
		t.Fatal("runWorker() error = nil, want the failed install surfaced")
	}

	outcomePath, err := OutcomePath(appDir)
	if err != nil {
		t.Fatalf("OutcomePath: %v", err)
	}
	got, err := readOutcome(outcomePath)
	if err != nil {
		t.Fatalf("readOutcome: %v", err)
	}
	if got.OK {
		t.Fatalf("outcome.OK = true, want false: the installer never ran")
	}
	if got.Version != "1.2.3" {
		t.Fatalf("outcome.Version = %q, want %q", got.Version, "1.2.3")
	}
	if got.Error == "" {
		t.Fatal("outcome.Error is empty, want the reason the update failed")
	}
	if got.FinishedAt.IsZero() {
		t.Fatal("outcome.FinishedAt is zero")
	}
}
