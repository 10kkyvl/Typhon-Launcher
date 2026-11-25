package selfupdate

import (
	"errors"
	"testing"
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
