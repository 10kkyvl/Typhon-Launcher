package relocate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/platform"
)

// killedCopy is a move killed while copying, its part copy in the staging
// folder and its old folder still there.
func killedCopy(t *testing.T) (recoveryFixture, string) {
	t.Helper()
	f := newRecoveryFixture(t, StageCopy, "")
	staging := f.target + stagingSuffix
	if err := os.RemoveAll(f.target); err != nil {
		t.Fatal(err)
	}
	mustMkdirFile(t, staging, "game.exe", "part")
	f.job.Staging = staging
	f.s = newRecoverTestService(t, f.job)
	f.s.lib = f.lib
	return f, staging
}

// Another game's removal, move or recovery holding the staging folder is a
// busy folder, not one in the way for good: the killed copy is left as it was
// for a later start, and that start deletes the part copy.
func TestKilledCopyWhoseStagingAnotherGameHoldsWaitsForIt(t *testing.T) {
	f, staging := killedCopy(t)
	other := addGameAt(t, f.lib, t.TempDir(), "Other")
	release, err := f.lib.BeginMove(other.ID, staging)
	if err != nil {
		t.Fatal(err)
	}

	f.s.recoverAll(context.Background())

	if got := f.s.soleJob(t); got.Stage != StageCopy || got.Error != f.job.Error {
		t.Fatalf("job = %s/%q %q, want it left for a later start", got.Stage, got.FailedAt, got.Error)
	}
	if !exists(staging) {
		t.Fatal("staging deleted while another game held it")
	}

	release()
	f.s.recoverAll(context.Background())
	if exists(staging) {
		t.Fatal("staging kept once nothing held it")
	}
}

// The hold recovery takes on the staging folder to delete it is let go: the
// game can be moved again in the same session.
func TestKilledCopyRecoveryLetsTheGameGo(t *testing.T) {
	f, staging := killedCopy(t)

	f.s.recoverAll(context.Background())

	if exists(staging) {
		t.Fatal("staging kept")
	}
	release, err := f.lib.BeginMove(f.game.ID, filepath.Join(t.TempDir(), "Game"))
	if err != nil {
		t.Fatalf("BeginMove after the recovery = %v, want the game free", err)
	}
	release()
}

// Without the game's reservation (the game left the library) recovery looks
// at the cards itself, and an uninstalled card, whose folder is only a hint
// for the next install, does not keep the part copy.
func TestKilledCopyOfARemovedGameIgnoresAnUninstalledCardInTheStaging(t *testing.T) {
	f, staging := killedCopy(t)
	hint := addExeAt(t, f.lib, filepath.Join(staging, "Old", "old.exe"), "Old")
	if err := f.lib.MarkUninstalled(hint.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.lib.RemoveGame(f.game.ID); err != nil {
		t.Fatal(err)
	}

	f.s.recoverAll(context.Background())

	if exists(staging) {
		t.Fatalf("part copy kept for an uninstalled card; jobs %+v", f.s.List())
	}
	if got := f.s.soleJob(t); strings.Contains(got.Error, "relocate.folder_shared") {
		t.Fatalf("job = %q, want no shared-folder refusal", got.Error)
	}
}

// A resumed cleanup that cannot walk the old folder (access denied, a drive
// gone mid-walk) deletes nothing and stays resumable.
func TestResumedCleanupThatCannotWalkTheOldFolderDeletesNothing(t *testing.T) {
	f := newRecoveryFixture(t, StageFailed, StageCleanup)
	if _, err := f.lib.Relocate(f.game.ID, f.target); err != nil {
		t.Fatal(err)
	}
	verifyTree = func(ctx context.Context, root string, m hashdir.Manifest, p func(hashdir.Progress)) (hashdir.Result, error) {
		if platform.SamePath(root, f.src) {
			return hashdir.Result{}, errors.New("walk failed: access denied")
		}
		return hashdir.Verify(ctx, root, m, p)
	}
	t.Cleanup(func() { verifyTree = hashdir.Verify })
	srcBefore := snapshotTree(t, f.src)

	f.s.recoverAll(context.Background())

	if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) {
		t.Fatal("old folder deleted although it could not be checked")
	}
	if got := f.s.soleJob(t); got.Stage != StageFailed || got.FailedAt != StageCleanup {
		t.Fatalf("job = %s/%q %q, want it resumable at the cleanup", got.Stage, got.FailedAt, got.Error)
	}
}

// A move refused at its journal write after the library granted its hold
// lets the game go again.
func TestMoveRefusedAtItsJournalWriteLetsTheGameGo(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	blockJournalDir(t, s, t.TempDir())

	if _, err := s.MoveGame(game.ID, t.TempDir()); err == nil {
		t.Fatal("MoveGame with an unwritable journal granted")
	}

	release, err := lib.BeginMove(game.ID, filepath.Join(t.TempDir(), "Game"))
	if err != nil {
		t.Fatalf("BeginMove after the refused move = %v, want the game free", err)
	}
	release()
	if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
		t.Fatal(err)
	}
}
