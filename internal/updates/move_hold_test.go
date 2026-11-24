package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/library"
)

// An update of a game being moved would read the folder the move deletes and
// register its version over it: it is refused before anything is downloaded,
// with the code the update card words, and the job slot is free again.
func TestStartUpdateRefusesAGameBeingMoved(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	h.library.moving = map[string]bool{"local-1": true}

	err := h.service.StartUpdate("local-1")

	if !errors.Is(err, errBusy) || !errors.Is(err, library.ErrGameRelocating) {
		t.Fatalf("StartUpdate during a move = %v, want updates.busy with the move as its cause", err)
	}
	if len(h.downloads.requests) != 0 {
		t.Fatalf("downloads started: %+v", h.downloads.requests)
	}
	if u, _ := h.service.snapshot("local-1"); u.State == StateUpdating || h.service.Busy("local-1") {
		t.Fatalf("update = %+v, busy = %v; want nothing started", u, h.service.Busy("local-1"))
	}
	if left := h.library.holdsLeft(); left != 0 {
		t.Fatalf("%d library holds left", left)
	}
}

// A repair writes the game's folder in place, as an update does.
func TestRepairRefusesAGameBeingMoved(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(damagedTorrentReport())
	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatal(err)
	}
	h.awaitJob(t, "local-1")
	h.library.moving = map[string]bool{"local-1": true}

	err := h.service.RepairGame("local-1")

	if !errors.Is(err, errBusy) || !errors.Is(err, library.ErrGameRelocating) {
		t.Fatalf("RepairGame during a move = %v, want updates.busy with the move as its cause", err)
	}
	if h.service.Busy("local-1") || len(h.downloads.requests) != 0 {
		t.Fatalf("repair started: busy = %v, downloads %+v", h.service.Busy("local-1"), h.downloads.requests)
	}
}

type registrationWatcher struct {
	*fakeLibrary
	heldAtRegistration int
}

func (w *registrationWatcher) ApplyInstalledUpdate(ctx context.Context, u library.InstalledUpdate) (library.Game, error) {
	w.heldAtRegistration = w.holdsLeft()
	return w.fakeLibrary.ApplyInstalledUpdate(ctx, u)
}

// The hold lasts the whole job: the version is registered under it, and it is
// let go when the job ends, whether it succeeded or failed.
func TestUpdateHoldsItsGameUntilTheJobEnds(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	watcher := &registrationWatcher{fakeLibrary: h.library}
	h.service.library = watcher

	h.downloads.mu.Lock()
	h.downloads.failTask = true
	h.downloads.mu.Unlock()
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateFailed)
	if left := h.library.holdsLeft(); left != 0 {
		t.Fatalf("%d library holds left after a failed update", left)
	}

	h.downloads.mu.Lock()
	h.downloads.failTask = false
	h.downloads.tasks = map[string]*download.Download{}
	h.downloads.mu.Unlock()
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateIdle)
	if watcher.heldAtRegistration != 1 {
		t.Fatalf("library holds at the registration = %d, want the update's own", watcher.heldAtRegistration)
	}
	if left := h.library.holdsLeft(); left != 0 {
		t.Fatalf("%d library holds left after the update", left)
	}
}

// The same on the real library: a move's hold refuses the update, and an
// update that ended (here refused by its plan) leaves the game free to move.
func TestUpdateAndMoveExcludeEachOtherOnTheRealLibrary(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Game")
	mkTree(t, current, "v1")
	lib, err := library.NewServiceAt(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	game, err := lib.RegisterInstalled(t.Context(), library.InstalledGame{Title: "Game", InstallDir: current, Executable: filepath.Join(current, "marker.txt"), Version: "1", ReleaseID: "r1", SourceID: "s", DistributionID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(filepath.Join(root, "updates"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.library = lib
	s.ctx = t.Context()
	downloads := newFakeDownloads()
	s.downloads = downloads
	s.updates[game.ID] = &Update{GameID: game.ID, Plan: &UpdatePlan{GameID: game.ID}}

	releaseMove, err := lib.BeginMove(game.ID, filepath.Join(t.TempDir(), "Game"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartUpdate(game.ID); !errors.Is(err, errBusy) || !errors.Is(err, library.ErrGameRelocating) {
		t.Fatalf("StartUpdate during the move = %v, want updates.busy", err)
	}
	releaseMove()

	if err := s.StartUpdate(game.ID); err == nil || errors.Is(err, library.ErrGameRelocating) {
		t.Fatalf("StartUpdate after the move = %v, want the plan's own refusal", err)
	}
	release, err := lib.BeginMove(game.ID, filepath.Join(t.TempDir(), "Game"))
	if err != nil {
		t.Fatalf("BeginMove after the update ended = %v", err)
	}
	release()
	if len(downloads.requests) != 0 {
		t.Fatalf("downloads started: %+v", downloads.requests)
	}
	if _, err := os.Stat(filepath.Join(current, "marker.txt")); err != nil {
		t.Fatal(err)
	}
}
