package relocate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"typhon/internal/library"
)

// The move asks the update service whether the game is busy before it takes
// the library's hold; an update that started after that answer holds the game
// in the library, and the hold refuses the move with the update's code,
// before anything is made at the target.
func TestMoveOfAGameAnUpdateHoldsIsRefused(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, fakeBusy{false}, fakeBusy{false})
	game, src := addTestGame(t, lib)
	release, err := lib.BeginUpdate(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	parent := t.TempDir()

	_, moveErr := s.MoveGame(game.ID, parent)

	if !errors.Is(moveErr, ErrUpdateBusy) {
		t.Fatalf("MoveGame during an update = %v, want ErrUpdateBusy", moveErr)
	}
	if jobs := s.List(); len(jobs) != 0 {
		t.Fatalf("jobs = %+v, want none", jobs)
	}
	if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
		t.Fatalf("source touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "Game")); err == nil {
		t.Fatal("target made despite the refusal")
	}
}

// A move's recovery meets an update of its game the same way: the job is left
// as it was for a later start, nothing is touched, and once the update is
// over the next recovery finishes the move.
func TestRecoveryOfAGameAnUpdateHoldsWaitsForIt(t *testing.T) {
	f := newRecoveryFixture(t, StageFailed, StageRepoint)
	release, err := f.lib.BeginUpdate(f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	srcBefore, targetBefore := snapshotTree(t, f.src), snapshotTree(t, f.target)

	f.s.recoverAll(context.Background())

	got := f.s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != StageRepoint || got.RecoveryAttempts != 0 {
		t.Fatalf("job = %s/%q attempts=%d, want it left as it was", got.Stage, got.FailedAt, got.RecoveryAttempts)
	}
	if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) || !reflect.DeepEqual(targetBefore, snapshotTree(t, f.target)) {
		t.Fatal("recovery touched the data while the update held the game")
	}
	card, err := f.lib.Find(f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !samePathT(card.InstallDir, f.src) {
		t.Fatalf("card repointed to %s during the update", card.InstallDir)
	}

	release()
	f.s.recoverAll(context.Background())
	card, err = f.lib.Find(f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !samePathT(card.InstallDir, f.target) || len(f.s.List()) != 0 {
		t.Fatalf("after the update: card at %s, jobs %+v; want the move finished", card.InstallDir, f.s.List())
	}
}

// The reverse: an update registered while a move holds its game is refused
// (P2b of the fifth review).
func TestUpdateRegisteredDuringALiveMoveIsRefused(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := t.TempDir()
	var applyErr error
	prev := renameDir
	renameDir = func(oldpath, newpath string) error {
		if err := prev(oldpath, newpath); err != nil {
			return err
		}
		if samePathT(oldpath, src) {
			mustMkdirFile(t, src, "game.exe", "version 2")
			_, applyErr = lib.ApplyInstalledUpdate(t.Context(), library.InstalledUpdate{ID: game.ID, InstallDir: src, Executable: filepath.Join(src, "game.exe"), Version: "2"})
		}
		return nil
	}
	t.Cleanup(func() { renameDir = prev })

	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatal(err)
	}
	done := s.wait(job.ID)

	if !errors.Is(applyErr, library.ErrGameMoving) {
		t.Fatalf("ApplyInstalledUpdate during the move = %v, want library.ErrGameMoving", applyErr)
	}
	card, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Version == "2" || !samePathT(card.InstallDir, filepath.Join(parent, "Game")) {
		t.Fatalf("card = %s v%q, move %s %q; want the moved game without the refused version", card.InstallDir, card.Version, done.Stage, done.Error)
	}
}

// And after a move finished: the update captured the old folder, and its
// registration must not bring the card back to it (P2 of the fifth review).
func TestUpdateRegisteredIntoTheOldFolderAfterAMoveIsRefused(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	save := mustMkdirFile(t, src, "save.dat", "progress")
	parent := t.TempDir()
	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatal(err)
	}
	if done := s.wait(job.ID); done.Stage != "" {
		t.Fatalf("move ended %s %q, want it finished", done.Stage, done.Error)
	}
	target := filepath.Join(parent, "Game")
	mustMkdirFile(t, src, "game.exe", "version 2")

	_, applyErr := lib.ApplyInstalledUpdate(t.Context(), library.InstalledUpdate{ID: game.ID, InstallDir: src, Executable: filepath.Join(src, "game.exe"), Version: "2"})

	if !errors.Is(applyErr, library.ErrFolderChanged) {
		t.Fatalf("ApplyInstalledUpdate into the old folder = %v, want library.ErrFolderChanged", applyErr)
	}
	card, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !samePathT(card.InstallDir, target) {
		t.Fatalf("card at %s, want the moved copy %s", card.InstallDir, target)
	}
	if _, err := os.Stat(filepath.Join(target, filepath.Base(save))); err != nil {
		t.Fatalf("moved saves: %v", err)
	}
}
