package selfupdate

import (
	"os"
	"sync/atomic"
	"testing"

	"typhon/internal/uierr"
)

// While an install, a removal, a move or a game update runs, the old
// launcher's shutdown would wait for it behind a hidden window: the hand-over
// is refused before anything is parked, with its own code, not the one that
// sends the update screen to the ordinary restart (which would refuse too).
func TestBeginHandoffIsRefusedWhileWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("the successor started while work runs") })
	r.readyForHandoff(t)
	var asked atomic.Int32
	r.svc.SetHandoffBlocker(func() bool {
		asked.Add(1)
		if _, ok := r.journal(t); ok {
			t.Error("the blocker was asked after the journal was written")
		}
		return true
	})

	err := r.svc.BeginHandoff(1)
	if uierr.Code(err) != "selfupdate.work_running" {
		t.Fatalf("BeginHandoff() = %v, want selfupdate.work_running", err)
	}
	if asked.Load() == 0 {
		t.Fatal("the blocker was never asked")
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	entries, rerr := os.ReadDir(r.disk.install)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries, want only the launcher", len(entries))
	}
	if _, ok := r.journal(t); ok {
		t.Fatal("a journal was written for a refused hand-over")
	}
	if st := r.svc.GetStatus(); st.State != StateReady || st.ErrorCode != "" || r.svc.HandoffInProgress() {
		t.Fatalf("status after the refusal = %+v, want ready without an error", st)
	}
}

// With nothing running the blocker lets the hand-over go ahead.
func TestBeginHandoffGoesAheadWhenNoWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	r.svc.SetHandoffBlocker(func() bool { return false })
	if _, err := r.svc.prepareHandoff(); err != nil {
		t.Fatalf("prepareHandoff() = %v, want the hand-over allowed", err)
	}
}

// The ordinary update quits the launcher and waits, in its shutdown, for the
// same work: the window froze on "the launcher will restart" and the worker
// gave up. It is refused with the same code before the worker is staged.
func TestApplyUpdateIsRefusedWhileWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("a successor started from the ordinary update") })
	var started atomic.Int32
	prevStart := startWorker
	startWorker = func(string, string) error {
		started.Add(1)
		return nil
	}
	t.Cleanup(func() { startWorker = prevStart })
	r.svc.SetHandoffBlocker(func() bool { return true })

	err := r.svc.ApplyUpdate("en")
	if uierr.Code(err) != "selfupdate.work_running" {
		t.Fatalf("ApplyUpdate() = %v, want selfupdate.work_running", err)
	}
	if started.Load() != 0 {
		t.Fatal("the worker was started while work runs")
	}
	workerDir, err := WorkerDir(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, workerDir) {
		t.Fatal("the worker was staged for a refused update")
	}
	specPath, err := SpecPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, specPath) {
		t.Fatal("an update spec was written for a refused update")
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	if st := r.svc.GetStatus(); st.State != StateReady || st.ErrorCode != "" {
		t.Fatalf("status after the refusal = %+v, want ready without an error", st)
	}

	// The refusal released the service: once the work is over the same
	// update goes ahead.
	r.svc.SetHandoffBlocker(func() bool { return false })
	if err := r.svc.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate() after the work ended = %v", err)
	}
	if started.Load() != 1 {
		t.Fatalf("the worker started %d times after the work ended, want once", started.Load())
	}
}
