package selfupdate

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"typhon/internal/uierr"
)

func (r *handoffRig) journal(t *testing.T) (handoffJournal, bool) {
	t.Helper()
	path, err := handoffJournalPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !exists(t, path) {
		return handoffJournal{}, false
	}
	j, err := loadJournal(r.disk.cfg)
	if err != nil {
		t.Fatalf("read the journal: %v", err)
	}
	return j, true
}

func stagingExists(t *testing.T, cfg string) bool {
	t.Helper()
	dir, err := stagingDir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return exists(t, dir)
}

// failRenamesFrom makes every rename from the n-th on fail, as a sharing
// violation would.
func failRenamesFrom(t *testing.T, n int) {
	t.Helper()
	prev := handoffRename
	calls := 0
	handoffRename = func(from, to string) error {
		calls++
		if calls >= n {
			return errors.New("sharing violation")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })
}

// The second rename of the swap fails and so does the swap's own undo: the
// running launcher stays parked under .typhon-old-* and nothing is at the
// launcher's name. Nothing may be swept and the journal must stay.
func TestFailedInSwapUndoKeepsTheJournalAndTheParkedLauncher(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("successor started") })
	failRenamesFrom(t, 2) // staged -> target, then every undo old -> target
	j := r.plan.journalFor(0)

	r.svc.runHandoff(t.Context(), r.plan, 1)

	if got := readFileString(t, j.OldPath); got != oldBody {
		t.Fatalf("parked launcher = %q, want the old build kept", got)
	}
	got, ok := r.journal(t)
	if !ok {
		t.Fatal("the journal that could restore the launcher is gone")
	}
	if got.Stage != stageUndoOwed {
		t.Errorf("journal stage = %q, want %q", got.Stage, stageUndoOwed)
	}
	st := r.svc.GetStatus()
	if st.State != StateReady || st.ErrorCode != errorCodeHandoffUndoFailed {
		t.Errorf("status = %+v, want ready with %q", st, errorCodeHandoffUndoFailed)
	}
	if r.svc.HandoffInProgress() {
		t.Error("the hand-off still reads as running")
	}
}

// The launcher's name is empty and the old build is parked: recovery puts it
// back, whatever version the journal was moving to.
func TestRecoverHandoffPaysAnOwedUndo(t *testing.T) {
	for _, current := range []string{"1.4.0", "1.5.0"} {
		t.Run(current, func(t *testing.T) {
			d := newHandoffDisk(t)
			plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
			j := plan.journalFor(1 << 22)
			if err := handoffRename(d.exe, j.OldPath); err != nil {
				t.Fatal(err)
			}
			j.Stage = stageUndoOwed
			if err := saveJournal(d.cfg, j); err != nil {
				t.Fatal(err)
			}
			prev := parentAlive
			parentAlive = func(int) (bool, error) { return false, nil }
			t.Cleanup(func() { parentAlive = prev })

			if err := RecoverHandoff(d.cfg, current); err != nil {
				t.Fatalf("RecoverHandoff() error = %v", err)
			}
			if got := readFileString(t, d.exe); got != oldBody {
				t.Fatalf("launcher = %q, want the parked old build back", got)
			}
			path, err := handoffJournalPath(d.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if exists(t, path) {
				t.Error("the journal survived a recovery that settled it")
			}
		})
	}
}

// A journal that says the new build took over while nothing is at the
// launcher's name: the parked launcher is the only one left and must not go.
func TestRecoverHandoffNeverSweepsTheOnlyLauncher(t *testing.T) {
	for _, stage := range []string{stageSwapped, stageReady, stageDone} {
		t.Run(stage, func(t *testing.T) {
			d := newHandoffDisk(t)
			plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
			j := plan.journalFor(1 << 22)
			if err := handoffRename(d.exe, j.OldPath); err != nil {
				t.Fatal(err)
			}
			j.Stage = stage
			if err := saveJournal(d.cfg, j); err != nil {
				t.Fatal(err)
			}
			prev := parentAlive
			parentAlive = func(int) (bool, error) { return false, nil }
			t.Cleanup(func() { parentAlive = prev })

			if err := RecoverHandoff(d.cfg, "1.5.0"); err != nil {
				t.Logf("RecoverHandoff() error = %v", err)
			}
			if !exists(t, d.exe) && !exists(t, j.OldPath) {
				t.Fatal("recovery deleted the only launcher")
			}
		})
	}
}

// readyForHandoff puts the rig's service where the update screen offers the
// update: ready, idle, with the rig's verified artifact, on a platform that
// hands over an executable.
func (r *handoffRig) readyForHandoff(t *testing.T) {
	t.Helper()
	prevMethod, prevTarget := methodFor, targetFor
	methodFor = func(Artifact, bool) handoffMethod { return methodExeFile }
	targetFor = func(exe string, _ handoffMethod) (string, error) { return exe, nil }
	t.Cleanup(func() { methodFor, targetFor = prevMethod, prevTarget })
	art := r.plan.Art
	r.svc.mu.Lock()
	r.svc.busy = false
	r.svc.status.State = StateReady
	r.svc.readyPath = r.plan.ArtifactPath
	r.svc.readyArtifact = &art
	r.svc.portable = true
	r.svc.mu.Unlock()
}

// countSnapshots replaces the snapshot of the running launcher with a record
// of what it would have saved, under which label.
func countSnapshots(t *testing.T) *[]string {
	t.Helper()
	var taken []string
	prev := handoffSaveCurrent
	handoffSaveCurrent = func(_ context.Context, _ string, exe, ver string) error {
		raw, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		taken = append(taken, ver+"="+string(raw))
		return nil
	}
	t.Cleanup(func() { handoffSaveCurrent = prev })
	return &taken
}

// failUndoOfAttempt makes putting the parked launcher of j back fail while
// fail reports true, as a successor that still holds its image would.
func failUndoOfAttempt(t *testing.T, j handoffJournal, fail *atomic.Bool) {
	t.Helper()
	prev := handoffRename
	handoffRename = func(from, to string) error {
		if from == j.OldPath && fail.Load() {
			return errors.New("access denied: image in use")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })
}

// After a failed undo the launcher's name holds the new build. A retry, by
// either path, would snapshot that build as the old version and sweep the
// first attempt's parked launcher, the only copy of the old build.
func TestRetryAfterAFailedUndoIsRefused(t *testing.T) {
	stubRollbackSupported(t, nil)
	taken := countSnapshots(t)
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {}) // never answers
	first := r.plan.journalFor(0)
	var fail atomic.Bool
	fail.Store(true)
	failUndoOfAttempt(t, first, &fail)
	r.svc.runHandoff(t.Context(), r.plan, 1)

	r.readyForHandoff(t)
	if err := r.svc.BeginHandoff(2); !errors.Is(err, ErrHandoffPending) {
		t.Fatalf("BeginHandoff() after a failed undo = %v, want ErrHandoffPending", err)
	}
	if got := uierr.Code(ErrHandoffPending); got != "selfupdate.handoff_pending" {
		t.Fatalf("code = %q", got)
	}
	r.readyForHandoff(t)
	if err := r.svc.ApplyUpdate("en"); !errors.Is(err, ErrHandoffPending) {
		t.Fatalf("ApplyUpdate() after a failed undo = %v, want ErrHandoffPending", err)
	}
	if len(*taken) != 1 || (*taken)[0] != "1.4.0="+oldBody {
		t.Fatalf("snapshots = %q, want only the first, of the old build", *taken)
	}
	if got := readFileString(t, first.OldPath); got != oldBody {
		t.Fatalf("the first attempt's parked launcher = %q, want the old build kept", got)
	}
	if st := r.svc.GetStatus(); st.State != StateReady || r.svc.HandoffInProgress() {
		t.Fatalf("status after the refusal = %+v", st)
	}
}

// Once the image is released the owed undo is paid on the next try, and the
// hand-over may go ahead from a launcher that is the old build again.
func TestRetryPaysTheOwedUndoFirst(t *testing.T) {
	stubRollbackSupported(t, nil)
	taken := countSnapshots(t)
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	first := r.plan.journalFor(0)
	var fail atomic.Bool
	fail.Store(true)
	failUndoOfAttempt(t, first, &fail)
	r.svc.runHandoff(t.Context(), r.plan, 1)
	fail.Store(false)

	r.readyForHandoff(t)
	plan, err := r.svc.prepareHandoff()
	if err != nil {
		t.Fatalf("prepareHandoff() after the image was released = %v", err)
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want the old build back before the retry", got)
	}
	if _, ok := r.journal(t); ok {
		t.Fatal("the settled journal is still there")
	}
	r.svc.runHandoff(t.Context(), plan, 2)
	if len(*taken) != 2 || (*taken)[1] != "1.4.0="+oldBody {
		t.Fatalf("snapshots = %q, want the retry to save the old build", *taken)
	}
}

func TestShutdownPaysTheOwedUndo(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("successor started") })
	j := r.plan.journalFor(0)
	var fail atomic.Bool
	fail.Store(true)
	failUndoOfAttempt(t, j, &fail)
	prev := handoffRename
	handoffRename = func(from, to string) error {
		if from == j.StagedPath {
			return errors.New("sharing violation")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })
	r.svc.runHandoff(t.Context(), r.plan, 1)
	if exists(t, r.disk.exe) {
		t.Fatal("setup: the launcher's name should be empty")
	}
	fail.Store(false)

	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q after shutdown, want the old build back", got)
	}
	if _, ok := r.journal(t); ok {
		t.Fatal("the journal survived the paid undo")
	}
}

// Windows: the killed successor still holds its image, so putting the old
// launcher back fails. The new build stays at the name, the old one parked.
func TestFailedUndoAfterTheSuccessorFailedKeepsTheParkedLauncher(t *testing.T) {
	stubRollbackSupported(t, nil)
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {}) // never answers
	j := r.plan.journalFor(0)
	prev := handoffRename
	handoffRename = func(from, to string) error {
		if from == j.OldPath && to == j.Target {
			return errors.New("access denied: image in use")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })

	r.svc.runHandoff(t.Context(), r.plan, 1)

	if got := readFileString(t, j.OldPath); got != oldBody {
		t.Fatalf("parked launcher = %q, want the old build kept", got)
	}
	if got, ok := r.journal(t); !ok || got.Stage != stageUndoOwed {
		t.Fatalf("journal = %+v (present %v), want stage %q", got, ok, stageUndoOwed)
	}
	if !stagingExists(t, r.disk.cfg) {
		t.Error("the snapshot of the old build was dropped while the undo is owed")
	}
}
