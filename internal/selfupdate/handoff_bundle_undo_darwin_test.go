//go:build darwin && !devmock

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// bundleDisk is an installed Typhon.app running the old build, the verified
// download of the new one and a config directory.
type bundleDisk struct {
	cfg, bundle, exe string
	plan             handoffPlan
}

func newBundleDisk(t *testing.T) bundleDisk {
	t.Helper()
	d := bundleDisk{cfg: t.TempDir(), bundle: filepath.Join(t.TempDir(), "Typhon.app")}
	d.exe = filepath.Join(d.bundle, "Contents", "MacOS", "typhon")
	writeTestFile(t, d.exe, []byte("old build"))
	//nolint:gosec // G302: the stand-in launcher must be executable, as a real one is
	if err := os.Chmod(d.exe, 0o755); err != nil {
		t.Fatal(err)
	}
	d.plan = handoffPlan{
		Method: methodBundle, Exe: d.exe, Target: d.bundle, ArtifactPath: appZip(t, "new build"),
		Nonce: "n0nce", Version: "1.5.0", FromVersion: "1.4.0",
	}
	return d
}

// lockInside puts a folder RemoveAll cannot empty into the bundle at path, so
// that removing that bundle stops half-way, and returns the folder's name
// relative to the bundle.
func lockInside(t *testing.T, bundle string) string {
	t.Helper()
	rel := filepath.Join("Contents", "Locked")
	writeTestFile(t, filepath.Join(bundle, rel, "keep"), []byte("x"))
	//nolint:gosec // G302: a folder that can be listed but not emptied, so that RemoveAll stops half-way
	if err := os.Chmod(filepath.Join(bundle, rel), 0o500); err != nil {
		t.Fatal(err)
	}
	return rel
}

func unlock(t *testing.T, dir string) {
	t.Helper()
	//nolint:gosec // G302: a folder needs its search bit for its files to be removed
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// assertOldBundleRuns checks that the launcher's bundle holds the old build
// and can be started.
func assertOldBundleRuns(t *testing.T, d bundleDisk) {
	t.Helper()
	raw, err := os.ReadFile(d.exe)
	if err != nil {
		t.Fatalf("the launcher's executable is gone: %v", err)
	}
	if string(raw) != "old build" {
		t.Fatalf("the launcher's executable = %q, want the old build", raw)
	}
	info, err := os.Stat(d.exe)
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the old build cannot be started: %v %v", info, err)
	}
}

// The exchange back succeeds, removing the new bundle stops half-way and the
// undo is owed. Retrying it, as the shutdown and the next attempt do, must
// not exchange again: the parked name holds the old bundle by then.
func TestBundleUndoRetriedAfterAPartialRemovalKeepsTheOldBundle(t *testing.T) {
	d := newBundleDisk(t)
	j := d.plan.journalFor(os.Getpid())
	undo, err := performSwap(t.Context(), d.plan, j)
	if err != nil {
		t.Fatalf("performSwap() = %v", err)
	}
	locked := lockInside(t, d.bundle) // the new bundle, at the target now
	t.Cleanup(func() {
		for _, b := range []string{d.bundle, j.OldPath} {
			//nolint:gosec // G302: a folder needs its search bit for t.TempDir to remove it
			if err := os.Chmod(filepath.Join(b, locked), 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Logf("unlock %s: %v", b, err)
			}
		}
	})

	if err := undo(); err == nil {
		t.Fatal("the first undo reported success although the new bundle was not removed")
	}
	assertOldBundleRuns(t, d)
	for i := 0; i < 2; i++ {
		if err := undo(); err == nil {
			t.Fatalf("retry %d reported success while the new bundle is still parked", i+1)
		}
		assertOldBundleRuns(t, d)
		if !exists(t, j.OldPath) {
			t.Fatalf("retry %d removed the parked name without the new bundle being gone", i+1)
		}
	}

	unlock(t, filepath.Join(j.OldPath, locked))
	if err := undo(); err != nil {
		t.Fatalf("the undo once the new bundle can go = %v", err)
	}
	assertOldBundleRuns(t, d)
	if exists(t, j.OldPath) {
		t.Fatal("the parked new bundle was left")
	}
}

// The undo may not act on a parked name it cannot identify: if the bundle at
// the launcher's name is not the one this launcher ran from, and neither is
// the parked one, nothing is exchanged and nothing removed.
func TestBundleUndoWithoutProofRemovesNothing(t *testing.T) {
	d := newBundleDisk(t)
	j := d.plan.journalFor(os.Getpid())
	undo, err := performSwap(t.Context(), d.plan, j)
	if err != nil {
		t.Fatalf("performSwap() = %v", err)
	}
	// Something replaced the parked old bundle with a copy of it: same bytes,
	// another bundle.
	stranger := filepath.Join(filepath.Dir(d.bundle), "Stranger.app")
	if err := os.Rename(j.OldPath, stranger); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(j.OldPath, "Contents", "MacOS", "typhon"), []byte("old build"))

	if err := undo(); err == nil {
		t.Fatal("undo() = nil for bundles it cannot tell apart")
	}
	if got := readFileString(t, d.exe); got != "new build" {
		t.Fatalf("the launcher's bundle holds %q, want it left alone", got)
	}
	if got := readFileString(t, filepath.Join(j.OldPath, "Contents", "MacOS", "typhon")); got != "old build" {
		t.Fatalf("the parked name holds %q, want it left alone", got)
	}
}

// Once the undo has exchanged the bundles back it never exchanges them
// again: if they are found exchanged once more, something other than this
// launcher moved them, and the retry leaves both where they are.
func TestBundleUndoNeverExchangesTwice(t *testing.T) {
	d := newBundleDisk(t)
	j := d.plan.journalFor(os.Getpid())
	undo, err := performSwap(t.Context(), d.plan, j)
	if err != nil {
		t.Fatal(err)
	}
	locked := lockInside(t, d.bundle)
	t.Cleanup(func() {
		for _, b := range []string{d.bundle, j.OldPath} {
			//nolint:gosec // G302: a folder needs its search bit for t.TempDir to remove it
			if err := os.Chmod(filepath.Join(b, locked), 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Logf("unlock %s: %v", b, err)
			}
		}
	})
	if err := undo(); err == nil {
		t.Fatal("the first undo reported success although the new bundle was not removed")
	}
	if err := unix.RenamexNp(j.OldPath, d.bundle, unix.RENAME_SWAP); err != nil {
		t.Fatal(err)
	}

	if err := undo(); err == nil {
		t.Fatal("the retry exchanged the bundles a second time")
	}
	if got := readFileString(t, filepath.Join(j.OldPath, "Contents", "MacOS", "typhon")); got != "old build" {
		t.Fatalf("the parked name holds %q, want the old bundle left there", got)
	}
}

// An undo that has finished is harmless to repeat.
func TestBundleUndoRepeatedAfterItFinishedChangesNothing(t *testing.T) {
	d := newBundleDisk(t)
	j := d.plan.journalFor(os.Getpid())
	undo, err := performSwap(t.Context(), d.plan, j)
	if err != nil {
		t.Fatal(err)
	}
	if err := undo(); err != nil {
		t.Fatalf("undo() = %v", err)
	}
	assertOldBundleRuns(t, d)
	if err := undo(); err != nil {
		t.Fatalf("a repeated undo with nothing parked = %v", err)
	}
	assertOldBundleRuns(t, d)
}

// The same failure through the service: the hand-over fails, its undo stops
// half-way, the shutdown retries it, and the next start settles the journal.
// The old bundle is at the launcher's name and runs at every step.
func TestBundleHandoffWithAHalfRemovedNewBundleKeepsTheOldOneThroughShutdownAndRestart(t *testing.T) {
	silenceSeams(t)
	stubRollbackSupported(t, errors.New("no rollback in this test"))
	d := newBundleDisk(t)
	j := d.plan.journalFor(os.Getpid())
	var locked string
	prevStart, prevTimeout := startHandoff, handoffReadyTimeout
	startHandoff = func(string, []string) (handoffProc, error) {
		locked = lockInside(t, d.bundle) // the new bundle is at the target now
		return newFakeSuccessor(), nil   // and never shows its window
	}
	handoffReadyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { startHandoff, handoffReadyTimeout = prevStart, prevTimeout })
	t.Cleanup(func() {
		for _, b := range []string{d.bundle, j.OldPath} {
			//nolint:gosec // G302: a folder needs its search bit for t.TempDir to remove it
			if err := os.Chmod(filepath.Join(b, "Contents", "Locked"), 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Logf("unlock %s: %v", b, err)
			}
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	svc := &Service{
		dir: d.cfg, store: mustStore(t, d.cfg), currentVersion: "1.4.0", ctx: ctx, cancel: cancel,
		status: Status{State: StateReady, CurrentVersion: "1.4.0", AvailableVersion: "1.5.0"},
	}

	svc.runHandoff(t.Context(), d.plan, 1)
	if st := svc.GetStatus(); st.ErrorCode != errorCodeHandoffUndoFailed {
		t.Fatalf("status = %+v, want %q", st, errorCodeHandoffUndoFailed)
	}
	assertOldBundleRuns(t, d)

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	assertOldBundleRuns(t, d)
	if !exists(t, j.OldPath) {
		t.Fatal("the shutdown removed the parked name while the new bundle could not go")
	}
	got, err := loadJournal(d.cfg)
	if err != nil || got.Stage != stageUndoOwed {
		t.Fatalf("journal after the shutdown = %+v, %v; want it kept as %q", got, err, stageUndoOwed)
	}

	// The next start, from the launcher's own bundle.
	unlock(t, filepath.Join(j.OldPath, locked))
	prevExe, prevAlive := currentExecutable, parentAlive
	currentExecutable = func() (string, error) { return d.exe, nil }
	parentAlive = func(int) (bool, error) { return false, nil }
	t.Cleanup(func() { currentExecutable, parentAlive = prevExe, prevAlive })
	if err := RecoverHandoff(d.cfg, "1.4.0"); err != nil {
		t.Fatalf("RecoverHandoff() = %v", err)
	}
	assertOldBundleRuns(t, d)
	if exists(t, j.OldPath) {
		t.Fatal("the parked new bundle was left after the restart")
	}
	path, err := handoffJournalPath(d.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, path) {
		t.Fatal("the settled journal was left")
	}
}
