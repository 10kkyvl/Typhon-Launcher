package relocate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/library"
	"typhon/internal/uierr"
)

// orphanRig is the next start after a move killed at its commit whose game
// then left the library: the staging folder holds the copy the job lists.
type orphanRig struct {
	s       *Service
	lib     *library.Service
	parent  string
	staging string
	kept    string
}

func newOrphanRig(t *testing.T, keepSource bool) orphanRig {
	t.Helper()
	lib := newTestLibrary(t)
	gone := addGameAt(t, lib, t.TempDir(), "Game")
	parent := t.TempDir()
	target := filepath.Join(parent, "Game")
	staging := target + stagingSuffix
	kept := mustMkdirFile(t, staging, "only-copy.bin", "the orphaned copy")
	if err := lib.RemoveGame(gone.ID); err != nil {
		t.Fatal(err)
	}
	if !keepSource {
		if err := os.RemoveAll(gone.InstallDir); err != nil {
			t.Fatal(err)
		}
	}
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: gone.ID, Title: "Game", Source: gone.InstallDir, Target: target, Staging: staging}
	s := newRecoverTestService(t, job)
	s.lib = lib
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx, s.cancel = ctx, cancel
	t.Cleanup(func() {
		cancel()
		s.wg.Wait()
	})
	s.recoverAll(ctx)
	if job := s.soleJob(t); !job.Orphaned {
		t.Fatalf("job after recovery = %+v, want it kept as an orphan", job)
	}
	return orphanRig{s: s, lib: lib, parent: parent, staging: staging, kept: kept}
}

func (r orphanRig) requireCopyListed(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(r.kept); err != nil {
		t.Fatalf("the orphaned copy was deleted or replaced: %v", err)
	}
	copies, err := r.s.OrphanedCopies()
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 1 || !samePathT(copies[0].Path, r.staging) {
		t.Fatalf("OrphanedCopies = %+v, want the staging folder %s", copies, r.staging)
	}
}

// Another game with the same folder name is moved into the same parent: its
// staging folder would be the orphan's, the only copy of the game.
func TestOrphanedStagingIsNotTakenByAMoveOfAnotherGame(t *testing.T) {
	forceCopyPath(t)
	r := newOrphanRig(t, false)
	other := addGameAt(t, r.lib, t.TempDir(), "Game")

	moved, err := r.s.MoveGame(other.ID, r.parent)
	if err == nil {
		done := r.s.wait(moved.ID)
		if _, statErr := os.Stat(r.kept); statErr != nil {
			t.Fatalf("a move of another game into the orphan's target (ended %s: %s) deleted its copy: %v", done.Stage, done.Error, statErr)
		}
		t.Fatalf("MoveGame into the orphan's target started; it ended %s (%s)", done.Stage, done.Error)
	}
	if code := uierr.Code(err); code != "relocate.target_not_empty" {
		t.Fatalf("MoveGame = %q (%v), want relocate.target_not_empty", code, err)
	}
	r.requireCopyListed(t)
}

// A staging folder no journaled move names is still not the move's to delete.
func TestMoveNeverDeletesAStagingFolderItDidNotCreate(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)
	parent := t.TempDir()
	kept := mustMkdirFile(t, filepath.Join(parent, "Game"+stagingSuffix), "someone-else.bin", "not the move's")

	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatalf("MoveGame = %v", err)
	}
	done := s.wait(job.ID)
	if done.Stage != StageFailed {
		t.Fatalf("move = %s (%s), want it failed on the occupied staging folder", done.Stage, done.Error)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("the move deleted a staging folder it did not create: %v", err)
	}
	if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, game.InstallDir) {
		t.Fatalf("card = %+v, %v; want it where it was", card, err)
	}
}

// The disk usage screen lists an orphan's copy once, under OrphanedCopies,
// and never as a leftover staging folder it may delete, whether or not the
// player kept the old folder.
func TestOrphanedStagingIsListedOnlyAsAnOrphanedCopy(t *testing.T) {
	for _, keepSource := range []bool{false, true} {
		name := "source deleted"
		if keepSource {
			name = "source kept"
		}
		t.Run(name, func(t *testing.T) {
			r := newOrphanRig(t, keepSource)
			stagings, err := r.s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			for _, st := range stagings {
				if samePathT(st.Path, r.staging) {
					t.Fatalf("Stagings lists the orphan's copy: %+v", st)
				}
			}
			if _, err := r.s.ReserveStaging(r.staging); uierr.Code(err) != "relocate.staging_refused" {
				t.Fatalf("ReserveStaging of the orphan's copy = %v, want relocate.staging_refused", err)
			}
			r.requireCopyListed(t)
		})
	}
}

// At the cleanup the card names the target and the old folder is the copy the
// cleanup was to delete: once the target is gone too, it is the copy left. Up
// to the repoint the card may still have named the old folder, which is then
// the player's own, kept or deleted by the player's removal, and never listed.
func TestOrphanedMoveListsTheOldFolderOnlyAtTheCleanup(t *testing.T) {
	cases := []struct {
		name     string
		stage    Stage
		failedAt Stage
		listed   bool
	}{
		{"cleanup", StageCleanup, "", true},
		{"failed cleanup", StageFailed, StageCleanup, true},
		{"repoint", StageRepoint, "", false},
		{"failed repoint", StageFailed, StageRepoint, false},
		{"commit", StageCommit, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			if err := lib.RemoveGame(game.ID); err != nil {
				t.Fatal(err)
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, FailedAt: tc.failedAt, GameID: game.ID, Title: "Game", Source: src, Target: target}
			if tc.stage == StageCommit {
				job.Staging = target + stagingSuffix
			}
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			if job := s.soleJob(t); !job.Orphaned {
				t.Fatalf("job = %+v, want it kept as an orphan", job)
			}
			copies, err := s.OrphanedCopies()
			if err != nil {
				t.Fatal(err)
			}
			if !tc.listed {
				if len(copies) != 0 {
					t.Fatalf("OrphanedCopies = %+v, want none: the player's own folder is no copy of the move", copies)
				}
				s.recoverAll(context.Background())
				if jobs := s.List(); len(jobs) != 0 {
					t.Fatalf("jobs = %+v, want the entry dropped", jobs)
				}
				return
			}
			if len(copies) != 1 || !samePathT(copies[0].Path, src) {
				t.Fatalf("OrphanedCopies = %+v, want the old folder %s", copies, src)
			}
			if err := os.RemoveAll(src); err != nil {
				t.Fatal(err)
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("job kept after its last copy is gone: %+v", jobs)
			}
		})
	}
}
