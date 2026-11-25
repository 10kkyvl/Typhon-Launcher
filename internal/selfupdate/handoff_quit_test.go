package selfupdate

import (
	"errors"
	"sync/atomic"
	"testing"

	"typhon/internal/handoffgate"
	"typhon/internal/uierr"
)

func countQuits(s *Service) *atomic.Int32 {
	var quits atomic.Int32
	s.SetSuccessorQuit(func() { quits.Add(1) })
	return &quits
}

func assertNotWaiting(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrNotWaiting) || uierr.Code(err) != "selfupdate.not_waiting" {
		t.Fatalf("QuitWaitingSuccessor() = %v, want selfupdate.not_waiting", err)
	}
}

// The player gives up on an old launcher that does not exit: the successor
// quits, once, and leaves the journal and the old launcher to the next start.
func TestQuitWaitingSuccessorQuitsOnceWhileTheOldLauncherRuns(t *testing.T) {
	r := newSuccessorRig(t, false)
	quits := countQuits(r.svc)
	journal := writeBrokenJournal(t, r.svc.dir) // any bytes: they must stay as they are
	asked := 0
	parentAlive = func(int) (bool, error) { asked++; return true, nil }

	for i := 0; i < 2; i++ {
		if err := r.svc.QuitWaitingSuccessor(); err != nil {
			t.Fatalf("QuitWaitingSuccessor() #%d = %v", i+1, err)
		}
	}
	if got := quits.Load(); got != 1 {
		t.Fatalf("quit %d times, want once", got)
	}
	if got := readFileString(t, journal); got != brokenJournal {
		t.Fatalf("journal = %q, want it left for the next start", got)
	}
	if asked != 0 {
		t.Fatal("the quit looked at the old launcher")
	}
}

func TestQuitWaitingSuccessorRefusesInAnOrdinaryStart(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	quits := countQuits(s)
	assertNotWaiting(t, s.QuitWaitingSuccessor())
	if quits.Load() != 0 {
		t.Fatal("an ordinary launcher quit")
	}
}

func TestQuitWaitingSuccessorRefusesInTheOldLauncher(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	quits := countQuits(r.svc)
	r.svc.runHandoff(t.Context(), r.plan, 1)
	assertNotWaiting(t, r.svc.QuitWaitingSuccessor())
	if quits.Load() != 0 {
		t.Fatal("the old launcher quit through the successor's call")
	}
}

// The old launcher exits while the successor the player closed is shutting
// down: the successor does not take over on its way out; the next start
// settles the hand-over.
func TestQuitWaitingSuccessorDoesNotTakeOverOnItsWayOut(t *testing.T) {
	r := newSuccessorRig(t, false)
	countQuits(r.svc)
	journal := writeBrokenJournal(t, r.svc.dir)
	if err := r.svc.QuitWaitingSuccessor(); err != nil {
		t.Fatalf("QuitWaitingSuccessor() = %v", err)
	}
	r.oldAlive.Store(false)
	tookOver := false
	if err := r.svc.CompleteHandoff(t.Context(), func() error { tookOver = true; return nil }); err != nil {
		t.Fatalf("CompleteHandoff() = %v, want a quiet return", err)
	}
	if tookOver || !handoffgate.Armed() {
		t.Fatalf("took over = %v, gate armed = %v: a quitting successor took over", tookOver, handoffgate.Armed())
	}
	if got := readFileString(t, journal); got != brokenJournal {
		t.Fatalf("journal = %q, want it left for the next start", got)
	}
}

func TestQuitWaitingSuccessorRefusesAfterTheHandoffIsDone(t *testing.T) {
	r := newSuccessorRig(t, false)
	quits := countQuits(r.svc)
	r.oldAlive.Store(false)
	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
	assertNotWaiting(t, r.svc.QuitWaitingSuccessor())
	if quits.Load() != 0 {
		t.Fatal("a successor that took over quit through the waiting call")
	}
}
