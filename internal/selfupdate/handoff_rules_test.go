package selfupdate

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/handoffgate"
	"typhon/internal/windowhandoff"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The successor may take the single-instance role and open the gate only
// once the old process has exited: two launchers would otherwise run the
// loops that write the same state.
func TestCompleteHandoffTakesOverOnlyAfterTheOldLauncherExited(t *testing.T) {
	r := newSuccessorRig(t, false)
	polled := make(chan struct{}, 1)
	parentAlive = func(int) (bool, error) {
		select {
		case polled <- struct{}{}:
		default:
		}
		return r.oldAlive.Load(), nil
	}
	var tookOverAlive atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- r.svc.CompleteHandoff(t.Context(), func() error {
			tookOverAlive.Store(r.oldAlive.Load())
			return nil
		})
	}()
	select {
	case <-polled:
	case err := <-done:
		t.Fatalf("CompleteHandoff() returned %v without asking whether the old launcher is alive", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the old launcher was never polled")
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
	if tookOverAlive.Load() {
		t.Fatal("the single instance was taken over while the old launcher was alive")
	}
}

// The old launcher shuts down slowly (services flush, a network times out).
// Killing it would cut those saves short and stop installers in its job, so
// the successor keeps waiting past the bound instead of quitting.
func TestSuccessorKeepsWaitingForASlowOldLauncher(t *testing.T) {
	r := newSuccessorRig(t, false)
	prevTimeout := parentExitTimeout
	parentExitTimeout = 20 * time.Millisecond
	t.Cleanup(func() { parentExitTimeout = prevTimeout })
	polls := make(chan struct{}, 64)
	parentAlive = func(int) (bool, error) {
		select {
		case polls <- struct{}{}:
		default:
		}
		return r.oldAlive.Load(), nil
	}
	done := make(chan error, 1)
	go func() { done <- r.complete(t) }()
	// Well past the bound: more polls than fit in it.
	for i := 0; i < 12; i++ {
		select {
		case <-polls:
		case err := <-done:
			t.Fatalf("CompleteHandoff() gave up with %v while the old launcher was still shutting down", err)
		case <-time.After(5 * time.Second):
			t.Fatal("the old launcher is no longer polled")
		}
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
}

// Once the successor's window is up, the old one goes at once and without a
// transition, before the old process starts its shutdown.
func TestOldLauncherHidesItsWindowBeforeItQuits(t *testing.T) {
	r := newHandoffRig(t, func(spec HandoffSpec, _ *fakeSuccessor) {
		if err := os.WriteFile(spec.ReadyPath, []byte(spec.Nonce), 0o600); err != nil { //nolint:forbidigo // test double for the successor's marker
			t.Errorf("write the marker: %v", err)
		}
	})
	r.svc.SetHandoffWindow(&application.WebviewWindow{})
	prevCapture := captureGeometry
	captureGeometry = func(*application.WebviewWindow) windowhandoff.Geometry {
		return windowhandoff.Geometry{Width: 800, Height: 600}
	}
	t.Cleanup(func() { captureGeometry = prevCapture })
	var hidden atomic.Int32
	hideOldWindow = func(*application.WebviewWindow) error {
		if r.quit.Load() != 0 {
			t.Error("the window was hidden after the quit")
		}
		hidden.Add(1)
		return nil
	}
	r.svc.runHandoff(t.Context(), r.plan, 1)
	r.waitQuit(t)
	if hidden.Load() != 1 {
		t.Fatalf("hidden %d times, want once before the quit", hidden.Load())
	}
}

// A successor keeps the stored update on disk until the old launcher is gone:
// if the hand-over is rolled back, the old one still offers that update.
func TestSuccessorStartupDoesNotSaveTheClearedState(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "1.5.0", CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cache, err := CacheDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache, "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.5.0"}
	s.SetHandoff(HandoffSpec{Nonce: "abc", ParentPID: 4321, Version: "1.5.0"})
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)

	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() = %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown() = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("state.json after a successor's start = %s\nwant it untouched: %s", after, before)
	}
}

// I-5: the journal is on disk before the first rename of the launcher.
func TestHandoffWritesTheJournalBeforeTheSwap(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	prev := handoffRename
	checked := false
	handoffRename = func(from, to string) error {
		if !checked {
			checked = true
			j, ok := r.journal(t)
			if !ok || j.Stage != stagePrepared || j.OldPath != to {
				t.Errorf("first rename %s -> %s with journal %+v (present %v), want stage prepared naming the parked path", from, to, j, ok)
			}
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })
	r.svc.runHandoff(t.Context(), r.plan, 1)
	if !checked {
		t.Fatal("the swap never renamed the launcher")
	}
}

// A successor that dies is noticed at once, not when the ready timeout ends.
func TestHandoffNoticesADeadSuccessorBeforeTheTimeout(t *testing.T) {
	r := newHandoffRig(t, func(_ HandoffSpec, proc *fakeSuccessor) { close(proc.done) })
	handoffReadyTimeout = time.Hour
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.svc.runHandoff(t.Context(), r.plan, 1)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the old launcher still waits for a successor that has exited")
	}
	r.assertRolledBack(t)
}

// BeginHandoff after ServiceShutdown has begun must not add a goroutine to
// the group shutdown waits for, nor touch the disk.
func TestBeginHandoffAfterShutdownStartsNothing(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("successor started after shutdown") })
	r.readyForHandoff(t)
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.BeginHandoff(1); err == nil {
		t.Fatal("BeginHandoff() after shutdown = nil")
	}
	if _, ok := r.journal(t); ok || readFileString(t, r.disk.exe) != oldBody {
		t.Fatal("a hand-over touched the disk after shutdown")
	}
	if r.svc.HandoffInProgress() {
		t.Fatal("a hand-over reads as running after shutdown")
	}
}

// Second launches that reach the old launcher during a hand-over are set
// aside; when the hand-over fails, the last one is acted on.
func TestALaunchHeldDuringAFailedHandoffIsReplayed(t *testing.T) {
	var r *handoffRig
	held := []bool{}
	r = newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {
		held = append(held, r.svc.HoldLaunch([]string{"typhon", "--first"}), r.svc.HoldLaunch([]string{"typhon", "--last"}))
	})
	if r.svc.HoldLaunch([]string{"typhon", "--before"}) {
		t.Fatal("a launch was held with no hand-over running")
	}
	replayed := make(chan []string, 2)
	r.svc.SetLaunchReplay(func(args []string) { replayed <- args })
	r.readyForHandoff(t)

	if err := r.svc.BeginHandoff(1); err != nil {
		t.Fatalf("BeginHandoff() = %v", err)
	}
	select {
	case args := <-replayed:
		if len(args) != 2 || args[1] != "--last" {
			t.Fatalf("replayed %q, want the last launch", args)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the held launch was not replayed after the hand-over failed")
	}
	if len(held) != 2 || !held[0] || !held[1] {
		t.Fatalf("HoldLaunch during the hand-over = %v, want both held", held)
	}
	select {
	case args := <-replayed:
		t.Fatalf("a second replay %q", args)
	default:
	}
	if r.svc.HoldLaunch([]string{"typhon", "--after"}) {
		t.Fatal("a launch was held after the hand-over failed")
	}
}
