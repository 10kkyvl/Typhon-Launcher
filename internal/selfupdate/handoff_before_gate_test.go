package selfupdate

import (
	"errors"
	"sync"
	"testing"

	"typhon/internal/handoffgate"
)

// steps records what ran, in order, from any goroutine.
type steps struct {
	mu   sync.Mutex
	list []string
}

func (s *steps) add(step string) {
	s.mu.Lock()
	s.list = append(s.list, step)
	s.mu.Unlock()
}

func (s *steps) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.list...)
}

// holdStart registers a start the gate holds back, as a service's
// ServiceStartup does in a successor.
func holdStart(t *testing.T, name string, ran *steps) {
	t.Helper()
	if err := handoffgate.Startup(t.Context(), name, func() error {
		ran.add("held start")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// The work set with SetBeforeGateOpens (the library's re-read) runs once the
// old launcher is gone and before the gate lets any held-back start run: the
// gate releases them all at once, and one of them may save the library at
// its first step.
func TestTheWorkBeforeTheGateRunsBeforeAnyHeldBackStart(t *testing.T) {
	r := newSuccessorRig(t, true)
	var ran steps
	holdStart(t, "test-before-gate-order", &ran)
	r.svc.SetBeforeGateOpens(func() error {
		if r.oldAlive.Load() {
			t.Error("the work before the gate ran while the old launcher was alive")
		}
		if !handoffgate.Armed() {
			t.Error("the work before the gate ran after the gate opened")
		}
		ran.add("before gate")
		return nil
	})
	r.oldAlive.Store(false)

	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
	got := ran.all()
	if len(got) != 2 || got[0] != "before gate" || got[1] != "held start" {
		t.Fatalf("ran %v, want the work before the gate, then the held start", got)
	}
}

// A failure of that work does not keep the launcher from coming up: the gate
// opens, the held-back starts run and the hand-over finishes.
func TestAFailedWorkBeforeTheGateStillOpensIt(t *testing.T) {
	r := newSuccessorRig(t, true)
	var ran steps
	holdStart(t, "test-before-gate-fails", &ran)
	r.svc.SetBeforeGateOpens(func() error {
		ran.add("before gate")
		return errors.New("library.json cannot be read")
	})
	r.oldAlive.Store(false)

	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v, want the hand-over finished", err)
	}
	if handoffgate.Armed() {
		t.Fatal("the gate stayed closed after the work before it failed")
	}
	if got := ran.all(); len(got) != 2 || got[1] != "held start" {
		t.Fatalf("ran %v, want the held start to run after the failed work", got)
	}
	if r.svc.HandoffInfo().Active {
		t.Fatal("the hand-over did not finish after the work before the gate failed")
	}
}
