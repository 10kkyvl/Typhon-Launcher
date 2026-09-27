package install

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestBaseContextRefusesBeforeServiceStartup closes finding 5:
// baseLocked/baseContext used to fall back to context.Background() when
// s.ctx was nil, a fallback invariant 20 forbids because ServiceShutdown can
// never cancel it. Before ServiceStartup there is no context to hand out at
// all, and the caller must see that as an error, not a context that works
// but never stops.
func TestBaseContextRefusesBeforeServiceStartup(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())

	ctx, err := s.baseContext()
	if !errors.Is(err, errNotStarted) {
		t.Fatalf("baseContext error = %v, want errNotStarted", err)
	}
	if ctx != nil {
		t.Fatalf("baseContext returned a context (%v) alongside an error", ctx)
	}
}

// TestSpawnLockedRefusesBeforeServiceStartupAndLeaksNothing closes finding 5
// for spawnLocked specifically: it must refuse to start the job goroutine
// rather than hand it an uncancellable context, must not register a job
// entry for work that never started, and must not leave s.wg counting a
// goroutine ServiceShutdown would then have to wait on forever.
func TestSpawnLockedRefusesBeforeServiceStartupAndLeaksNothing(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())

	s.mu.Lock()
	err := s.spawnLocked("job-1")
	_, hasJob := s.jobs["job-1"]
	s.mu.Unlock()

	if !errors.Is(err, errNotStarted) {
		t.Fatalf("spawnLocked error = %v, want errNotStarted", err)
	}
	if hasJob {
		t.Fatal("spawnLocked registered a job entry despite refusing to start it")
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("s.wg.Wait() never returned: spawnLocked left an untracked goroutine running")
	}
}

// TestStartRefusesBeforeServiceStartup exercises the same fix through the
// public entry point installs actually go through: Start must refuse rather
// than run Inspect and spawn a job on a service that was never started.
func TestStartRefusesBeforeServiceStartup(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	downloads := newFakeDownloads()
	s.downloads = downloads

	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Game")

	if _, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy}); !errors.Is(err, errNotStarted) {
		t.Fatalf("Start error = %v, want errNotStarted", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("Start left an item behind despite refusing: %+v", got)
	}
}
