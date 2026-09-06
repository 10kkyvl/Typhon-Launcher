package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
)

// blockingLibrary lets a test hold checkAll inside GetInstalledGames until it
// chooses to release it, so it can observe whether a goroutine that calls
// checkAll is still running.
type blockingLibrary struct {
	inner   librarySource
	release chan struct{}
	entered chan struct{}
}

func (b *blockingLibrary) GetInstalledGames() []library.Game {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	return b.inner.GetInstalledGames()
}

func (b *blockingLibrary) GetRunningGames() []string { return b.inner.GetRunningGames() }

func (b *blockingLibrary) ApplyInstalledUpdate(u library.InstalledUpdate) (library.Game, error) {
	return b.inner.ApplyInstalledUpdate(u)
}

func (b *blockingLibrary) LocateSaves(ctx context.Context, id string) (library.SavesResult, error) {
	return b.inner.LocateSaves(ctx, id)
}

// TestServiceShutdownWaitsForHandleSourcesRefreshedGoroutine covers
// invariant 19: HandleSourcesRefreshed used to start "go s.checkAll(...)"
// without registering it in s.wg, so ServiceShutdown's wg.Wait() could return
// (and run its own final persist) while that goroutine was still writing
// updates.json. Blocking the recheck inside GetInstalledGames lets the test
// see whether ServiceShutdown actually waits for it.
func TestServiceShutdownWaitsForHandleSourcesRefreshedGoroutine(t *testing.T) {
	h := newHarness(t)
	blocking := &blockingLibrary{inner: h.library, release: make(chan struct{}), entered: make(chan struct{}, 1)}
	h.service.library = blocking

	h.service.HandleSourcesRefreshed()

	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleSourcesRefreshed did not start a recheck")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- h.service.ServiceShutdown() }()

	select {
	case <-shutdownDone:
		t.Fatal("ServiceShutdown returned before the tracked recheck goroutine finished")
	case <-time.After(150 * time.Millisecond):
	}

	close(blocking.release)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServiceShutdown did not return after the recheck goroutine finished")
	}
}

// TestHandleSourcesRefreshedRefusesWhileClosing covers the other half of
// invariant 19: once ServiceShutdown has flipped s.closing, later calls must
// not start new work behind its back.
func TestHandleSourcesRefreshedRefusesWhileClosing(t *testing.T) {
	h := newHarness(t)
	h.service.mu.Lock()
	h.service.closing = true
	h.service.mu.Unlock()

	blocking := &blockingLibrary{inner: h.library, release: make(chan struct{}), entered: make(chan struct{}, 1)}
	close(blocking.release)
	h.service.library = blocking

	h.service.HandleSourcesRefreshed()

	select {
	case <-blocking.entered:
		t.Fatal("HandleSourcesRefreshed started a recheck while the service is closing")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestBeginJobRefusesWithoutStartedService covers invariant 20: beginJob must
// refuse (not fall back to context.Background()) when ServiceStartup has not
// run yet, since a job tied to no cancellable context could outlive a
// shutdown that never happened.
func TestBeginJobRefusesWithoutStartedService(t *testing.T) {
	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ctx, ok := svc.beginJob("g1"); ok || ctx != nil {
		t.Fatalf("beginJob = (%v, %v), want (nil, false) before ServiceStartup", ctx, ok)
	}
}

// TestStartUpdateRefusesWithoutStartedService confirms the beginJob refusal
// reaches a real caller as an ordinary busy error instead of a panic or a
// job silently running under context.Background().
func TestStartUpdateRefusesWithoutStartedService(t *testing.T) {
	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{{ID: "g1", InstallDir: t.TempDir()}}}
	svc.downloads = newFakeDownloads()
	svc.updates["g1"] = &Update{GameID: "g1", Plan: &UpdatePlan{}}

	if err := svc.StartUpdate("g1"); !errors.Is(err, errBusy) {
		t.Fatalf("StartUpdate() = %v, want errBusy before ServiceStartup", err)
	}
}

// TestCheckUpdatesSkipsCheckWithoutStartedService covers CheckUpdates, which
// cannot return an error (its signature is fixed by the wails binding): with
// no context yet it must skip the check entirely rather than run it under
// context.Background().
func TestCheckUpdatesSkipsCheckWithoutStartedService(t *testing.T) {
	dir := t.TempDir()
	svc, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{{ID: "g1", Title: "Game"}}}

	got := svc.CheckUpdates()
	if len(got) != 0 {
		t.Fatalf("CheckUpdates() = %+v, want none before ServiceStartup", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "updates.json")); err == nil {
		t.Fatal("CheckUpdates persisted state despite the service never starting")
	}
}
