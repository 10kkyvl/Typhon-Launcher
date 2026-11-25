package selfupdate

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"typhon/internal/uierr"
)

// While an install, a removal or an elevated installer runs, the old
// launcher's shutdown would wait for it behind a hidden window: the hand-over
// is refused before anything is parked, with the code that sends the update
// screen to the ordinary restart without a word.
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
	if !errors.Is(err, ErrHandoffUnsupported) || uierr.Code(err) != "selfupdate.handoff_unsupported" {
		t.Fatalf("BeginHandoff() = %v, want selfupdate.handoff_unsupported", err)
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
