package relocate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/hashdir"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// A move that is copying is work the launcher's shutdown cancels and waits
// for: the self-update must not quit the launcher under it. Before and after
// it nothing runs.
func TestWorkRunningWhileAMoveCopies(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)
	waitUntil(t, "the startup's recovery to end", func() bool { return !s.WorkRunning() })

	entered := make(chan struct{})
	release := make(chan struct{})
	prevRename := renameDir
	// Another drive: the move copies instead of renaming.
	renameDir = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errCrossDevice}
	}
	afterCopyBeforeVerify = func(string) {
		close(entered)
		<-release
	}
	t.Cleanup(func() { renameDir, afterCopyBeforeVerify = prevRename, nil })

	job, err := s.MoveGame(game.ID, t.TempDir())
	if err != nil {
		t.Fatalf("MoveGame: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the move did not reach its copy")
	}
	running := s.WorkRunning()
	close(release)
	if !running {
		t.Fatal("WorkRunning() = false while a move copies")
	}
	if got := s.wait(job.ID); got.Stage == StageFailed {
		t.Fatalf("setup: the move failed: %s", got.Error)
	}
	if s.WorkRunning() {
		t.Fatal("WorkRunning() = true after the move ended")
	}
}

// The startup's recovery of an interrupted move (here: the old copy's
// removal after a verified move) is work too, until it has ended.
func TestWorkRunningWhileAMoveIsRecovered(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	mustMkdirFile(t, src, "game.exe", "original")
	tgt := filepath.Join(root, "tgt")
	mustMkdirFile(t, tgt, "game.exe", "original")
	manifest, err := hashdir.Build(t.Context(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: itemScreenshots, Source: src, Target: tgt, TotalBytes: 8}
	st := newStore(dir)
	if err := st.saveJournal([]Job{job}); err != nil {
		t.Fatal(err)
	}
	if err := st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	prevRemove := removeTree
	removeTree = func(path string) error {
		close(entered)
		<-release
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { removeTree = prevRemove })

	s, err := NewServiceAt(dir, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("relocate shutdown: %v", err)
		}
	})
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the recovery did not reach the old copy's removal")
	}
	running := s.WorkRunning()
	close(release)
	if !running {
		t.Fatal("WorkRunning() = false while the startup recovers a move")
	}
	waitUntil(t, "the recovery to end", func() bool { return !s.WorkRunning() })
	if got := s.List(); len(got) != 0 {
		t.Fatalf("setup: the recovered move is still listed: %+v", got)
	}
}
