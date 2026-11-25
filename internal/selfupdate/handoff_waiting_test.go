package selfupdate

import (
	"testing"
	"time"

	"typhon/internal/handoffgate"
)

// waitUntil polls cond until it holds or the deadline passes.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !cond() {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// pollsOfTheOldLauncher counts the successor's checks of the old process.
func (r *successorRig) pollsOfTheOldLauncher() <-chan struct{} {
	polls := make(chan struct{}, 256)
	parentAlive = func(int) (bool, error) {
		select {
		case polls <- struct{}{}:
		default:
		}
		return r.oldAlive.Load(), nil
	}
	return polls
}

// The old launcher hid its window and is still shutting down. Some time after
// the successor's window came up, the update screen can say so: HandoffInfo
// carries waiting, for a page loaded later too. The gate stays closed, and
// once the old launcher is gone the hand-over finishes as before.
func TestSuccessorReportsAnOldLauncherSlowToExitAfterTheReveal(t *testing.T) {
	r := newSuccessorRig(t, false)
	prevSlow := handoffSlowAfter
	handoffSlowAfter = 20 * time.Millisecond
	t.Cleanup(func() { handoffSlowAfter = prevSlow })
	polls := r.pollsOfTheOldLauncher()
	done := make(chan error, 1)
	go func() { done <- r.complete(t) }()

	// Longer than handoffSlowAfter, but the window is not up yet: the wait
	// is counted from the reveal.
	start := time.Now()
	for time.Since(start) < 4*handoffSlowAfter {
		select {
		case <-polls:
		case err := <-done:
			t.Fatalf("CompleteHandoff() returned %v while the old launcher runs", err)
		case <-time.After(5 * time.Second):
			t.Fatal("the old launcher is no longer polled")
		}
	}
	if r.svc.HandoffInfo().Waiting {
		t.Fatal("waiting before the successor's window was revealed")
	}

	if err := r.svc.HandoffRendered(); err != nil {
		t.Fatalf("HandoffRendered() = %v", err)
	}
	waitUntil(t, "the slow old launcher to be reported", func() bool { return r.svc.HandoffInfo().Waiting })
	if info := r.svc.HandoffInfo(); !info.Active {
		t.Fatalf("HandoffInfo() = %+v, want still active while waiting", info)
	}
	if !handoffgate.Armed() {
		t.Fatal("the gate opened while the old launcher still runs")
	}

	r.oldAlive.Store(false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CompleteHandoff() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CompleteHandoff did not return after the old launcher exited")
	}
	if info := r.svc.HandoffInfo(); info.Active || info.Waiting {
		t.Fatalf("HandoffInfo() after the take-over = %+v, want neither active nor waiting", info)
	}
}

// An old launcher that exits promptly after the reveal is never reported.
func TestSuccessorDoesNotReportAPromptOldLauncher(t *testing.T) {
	r := newSuccessorRig(t, false)
	prevSlow := handoffSlowAfter
	handoffSlowAfter = time.Hour
	t.Cleanup(func() { handoffSlowAfter = prevSlow })
	polls := r.pollsOfTheOldLauncher()
	done := make(chan error, 1)
	go func() { done <- r.complete(t) }()
	if err := r.svc.HandoffRendered(); err != nil {
		t.Fatalf("HandoffRendered() = %v", err)
	}
	for i := 0; i < 5; i++ {
		<-polls
	}
	if r.svc.HandoffInfo().Waiting {
		t.Fatal("waiting reported before handoffSlowAfter")
	}
	r.oldAlive.Store(false)
	if err := <-done; err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
}
