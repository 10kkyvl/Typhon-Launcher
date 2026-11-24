package relocate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/hashdir"
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

// After the crash the player added a game found inside the old folder of an
// interrupted move. Recovery holds the move's folders as a live move does: it
// does not delete the old folder with that game in it. The job reads as failed
// at its stage with the reason, not as a move still running, spends no
// recovery attempt, and the next start retries it from there. The game's
// files stay when its card leaves, so the cleanup retried then still keeps
// the folder: it holds files the copy lacks.
func TestRecoveryFailsAMoveWhoseFolderGainedAnotherGame(t *testing.T) {
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "Game")
	mustMkdirFile(t, target, "game.exe", "stub")
	if _, err := lib.Relocate(game.ID, target); err != nil {
		t.Fatal(err)
	}
	bonus := filepath.Join(src, "Bonus", "Game2.exe")
	other := addExeAt(t, lib, bonus, "Bonus")
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: game.ID, Title: "Game", Source: src, Target: target}
	s := newRecoverTestService(t, job)
	s.lib = lib
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}

	s.recoverAll(context.Background())

	if _, err := os.Stat(bonus); err != nil {
		t.Fatalf("recovery deleted the old folder with another game in it: %v", err)
	}
	if got := s.soleJob(t); got.Stage != StageFailed || got.FailedAt != StageCleanup || got.RecoveryAttempts != 0 || !strings.Contains(got.Error, "relocate.folder_shared") {
		t.Fatalf("job = %s/%q attempts=%d error=%q, want it failed at the cleanup by relocate.folder_shared, no attempt spent", got.Stage, got.FailedAt, got.RecoveryAttempts, got.Error)
	}
	if _, err := lib.Find(other.ID); err != nil {
		t.Fatalf("card of the other game: %v", err)
	}
	if err := lib.RemoveGame(other.ID); err != nil {
		t.Fatal(err)
	}
	s.recoverAll(context.Background())
	if got := s.soleJob(t); got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.source_changed") {
		t.Fatalf("job after the other game left = %s/%q %q, want it failed for good by relocate.source_changed: the old folder holds files the copy lacks", got.Stage, got.FailedAt, got.Error)
	}
	if _, err := os.Stat(bonus); err != nil {
		t.Fatalf("recovery deleted files the copy lacks: %v", err)
	}
}

// A move killed while copying leaves a part copy in its staging folder, which
// recovery deletes. A game added inside that folder since (a scan, or a
// journal from before the live move held it) is never deleted with it: the job
// fails for good, the folder stays and is listed as one that holds a game.
func TestRecoveryAtTheCopyNeverDeletesAGameInTheStagingFolder(t *testing.T) {
	cases := []struct {
		name    string
		stage   Stage
		removed bool
	}{
		{"copy", StageCopy, false},
		{"verify", StageVerify, false},
		{"copy, the game removed since", StageCopy, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			staging := target + stagingSuffix
			mustMkdirFile(t, staging, "game.exe", "part")
			bonus := filepath.Join(staging, "Bonus", "Game2.exe")
			addExeAt(t, lib, bonus, "Bonus")
			if tc.removed {
				if err := lib.RemoveGame(game.ID); err != nil {
					t.Fatal(err)
				}
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			if _, err := os.Stat(bonus); err != nil {
				t.Fatalf("recovery deleted a game added inside the staging folder: %v", err)
			}
			got := s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.folder_shared") {
				t.Fatalf("job = %s/%q %q, want it failed for good by relocate.folder_shared", got.Stage, got.FailedAt, got.Error)
			}
			stagings, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if len(stagings) != 1 || stagings[0].Deletable {
				t.Fatalf("Stagings = %+v, want the folder listed and not deletable", stagings)
			}
		})
	}
}

// A tool added from the games folder itself holds that folder: a live move
// into it goes ahead, and so does the same move resumed after a kill.
func TestRecoveryFinishesAMoveIntoAFolderAnotherCardHolds(t *testing.T) {
	lib := newTestLibrary(t)
	parent := t.TempDir()
	addExeAt(t, lib, filepath.Join(parent, "tool.exe"), "Tool")

	live, _ := addTestGame(t, lib)
	ls := newTestService(t, nil, lib, nil, nil, nil)
	moved, err := ls.MoveGame(live.ID, parent)
	if err != nil {
		t.Fatalf("live MoveGame into a folder with a card = %v", err)
	}
	if done := ls.wait(moved.ID); done.Stage != "" && done.Stage != StageDone {
		t.Fatalf("live move = %s (%s)", done.Stage, done.Error)
	}
	if card, err := lib.Find(live.ID); err != nil || !samePathT(card.InstallDir, filepath.Join(parent, "Game")) {
		t.Fatalf("live move card = %+v, %v; want it moved into %s", card, err, parent)
	}

	game, src := addTestGame(t, lib)
	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "Game2")
	mustMkdirFile(t, target, "game.exe", "stub")
	if _, err := lib.Relocate(game.ID, target); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: game.ID, Title: "Game", Source: src, Target: target}
	s := newRecoverTestService(t, job)
	s.lib = lib
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}

	s.recoverAll(context.Background())

	if jobs := s.List(); len(jobs) != 0 {
		t.Fatalf("recovery of the killed move = %+v, want it finished like the live move", jobs)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("old folder after recovery: %v", err)
	}
}

// Being blocked by another card is not the move's failure: however many
// starts find the card in the way, the job spends no attempt and stays failed
// at its stage with relocate.folder_shared, to be resumed. A job killed at the
// repoint, or at the prepare after a same-volume rename landed, has no staging
// folder; its copy at the target (in the second case the only copy) stays
// tracked, and the move finishes once the card and its files are gone.
func TestRefusalByAnotherCardNeverRetiresTheMove(t *testing.T) {
	cases := []struct {
		name  string
		stage Stage
		// renamed: the same-volume rename had landed before the kill.
		renamed bool
	}{
		{"repoint", StageRepoint, false},
		{"prepare, rename landed", StagePrepare, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "Game")
			if tc.renamed {
				if err := os.Rename(src, target); err != nil {
					t.Fatal(err)
				}
			} else {
				mustMkdirFile(t, target, "game.exe", "stub")
			}
			bonus := filepath.Join(target, "Bonus")
			tool := addExeAt(t, lib, filepath.Join(bonus, "tool.exe"), "Bonus")
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, GameID: game.ID, Title: "Game", Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}

			for start := range maxRecoveryAttempts + 2 {
				s.recoverAll(context.Background())
				got := s.soleJob(t)
				if got.Stage != StageFailed || got.FailedAt != tc.stage || got.RecoveryAttempts != 0 || !strings.Contains(got.Error, "relocate.folder_shared") {
					t.Fatalf("start %d: job = %s/%q attempts=%d error=%q, want it failed at %s by relocate.folder_shared, no attempt spent", start+1, got.Stage, got.FailedAt, got.RecoveryAttempts, got.Error, tc.stage)
				}
			}
			if _, err := os.Stat(filepath.Join(target, "game.exe")); err != nil {
				t.Fatalf("the copy at the target: %v", err)
			}

			if err := lib.RemoveGame(tool.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(bonus); err != nil {
				t.Fatal(err)
			}
			s.recoverAll(context.Background())

			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("jobs after the card left = %+v, want the move finished", jobs)
			}
			if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, target) {
				t.Fatalf("card at %s (%v), want it at %s", card.InstallDir, err, target)
			}
			if _, err := os.Stat(src); !os.IsNotExist(err) {
				t.Fatalf("old folder after the move finished: %v", err)
			}
		})
	}
}

// A move killed at its commit with the copy still in the staging folder, and a
// card added inside that folder since: recovery renames nothing and fails the
// job at the commit; once that card and its files are gone the next start
// finishes the move from the commit.
func TestRecoveryFailsAtTheCommitAndResumesFromIt(t *testing.T) {
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "Game")
	staging := target + stagingSuffix
	mustMkdirFile(t, staging, "game.exe", "stub")
	tool := addExeAt(t, lib, filepath.Join(staging, "Tool", "tool.exe"), "Tool")
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
	s := newRecoverTestService(t, job)
	s.lib = lib
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}

	s.recoverAll(context.Background())

	got := s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != StageCommit || !strings.Contains(got.Error, "relocate.folder_shared") {
		t.Fatalf("job = %s/%q error=%q, want it failed at the commit by relocate.folder_shared", got.Stage, got.FailedAt, got.Error)
	}
	if _, err := os.Stat(filepath.Join(staging, "Tool", "tool.exe")); err != nil {
		t.Fatalf("the other card's files: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("the copy was renamed over although another card was inside it: %v", err)
	}

	if err := lib.RemoveGame(tool.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(staging, "Tool")); err != nil {
		t.Fatal(err)
	}
	s.recoverAll(context.Background())

	if jobs := s.List(); len(jobs) != 0 {
		t.Fatalf("jobs after the other card left = %+v, want the move finished", jobs)
	}
	if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, target) {
		t.Fatalf("card = %+v, %v; want it at %s", card, err, target)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("old folder after the move finished: %v", err)
	}
}

// Disk usage gets every copy an orphaned move left, each named for what it is,
// not only the first one found; the entry goes once none of them is left.
func TestOrphanedCopiesListsEveryCopyWithItsKind(t *testing.T) {
	type copyAt struct{ kind, path string }
	cases := []struct {
		name string
		// leave prepares the card and the folders and returns the job.
		leave func(t *testing.T, lib *library.Service, game library.Game, src, target string) Job
		want  func(src, target string) []copyAt
	}{
		{"cleanup with the target and the old folder", func(t *testing.T, lib *library.Service, game library.Game, src, target string) Job {
			mustMkdirFile(t, target, "game.exe", "stub")
			if err := lib.RemoveGame(game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: game.ID, Title: "Game", Source: src, Target: target}
		}, func(src, target string) []copyAt {
			return []copyAt{{"target", target}, {"source", src}}
		}},
		{"repoint that landed, then uninstalled", func(t *testing.T, lib *library.Service, game library.Game, src, target string) Job {
			mustMkdirFile(t, target, "game.exe", "stub")
			if _, err := lib.Relocate(game.ID, target); err != nil {
				t.Fatal(err)
			}
			if err := lib.MarkUninstalled(game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageRepoint, GameID: game.ID, Title: "Game", Source: src, Target: target}
		}, func(src, target string) []copyAt {
			return []copyAt{{"target", target}, {"source", src}}
		}},
		{"repoint that did not land, then uninstalled", func(t *testing.T, lib *library.Service, game library.Game, src, target string) Job {
			mustMkdirFile(t, target, "game.exe", "stub")
			if err := lib.MarkUninstalled(game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageRepoint, GameID: game.ID, Title: "Game", Source: src, Target: target}
		}, func(_, target string) []copyAt {
			return []copyAt{{"target", target}}
		}},
		{"commit with the copy in staging", func(t *testing.T, lib *library.Service, game library.Game, src, target string) Job {
			mustMkdirFile(t, target+stagingSuffix, "game.exe", "stub")
			if err := lib.RemoveGame(game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: target + stagingSuffix}
		}, func(_, target string) []copyAt {
			return []copyAt{{"staging", target + stagingSuffix}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			job := tc.leave(t, lib, game, src, target)
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			copies, err := s.OrphanedCopies()
			if err != nil {
				t.Fatal(err)
			}
			want := tc.want(src, target)
			if len(copies) != len(want) {
				t.Fatalf("OrphanedCopies = %+v, want %+v", copies, want)
			}
			for i, w := range want {
				if copies[i].Kind != w.kind || !samePathT(copies[i].Path, w.path) || copies[i].JobID != job.ID {
					t.Fatalf("OrphanedCopies[%d] = %+v, want %s at %s", i, copies[i], w.kind, w.path)
				}
			}
			for _, w := range want {
				if err := os.RemoveAll(w.path); err != nil {
					t.Fatal(err)
				}
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("job kept after its last copy is gone: %+v", jobs)
			}
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

// Each listed copy says what Disk usage needs to word and guard it: movedOn
// only while the game's card names another folder that is on disk; onlyCopy
// when the card names a folder that is gone, or the game has no card with
// files, and the copy is the newest complete one the move knows; holdsGame
// when a card's files lie at, in or around it.
func TestOrphanedCopySaysWhetherItIsTheOnlyCopy(t *testing.T) {
	type fact struct {
		kind                         string
		movedOn, onlyCopy, holdsGame bool
	}
	type rig struct {
		lib         *library.Service
		game        library.Game
		src, target string
	}
	cases := []struct {
		name string
		// leave prepares the card and folders and returns the job as the
		// journal has it before a start.
		leave func(t *testing.T, r rig) Job
		// after changes the disk once recovery has run.
		after func(t *testing.T, r rig)
		want  func(r rig) []fact
	}{
		{"renamed, out of attempts, card at the old folder that is gone", func(t *testing.T, r rig) Job {
			if err := os.Rename(r.src, r.target); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Renamed: true, RecoveryAttempts: maxRecoveryAttempts, Error: ErrRetriesExhausted.Error(), GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, false, true, false}} }},
		{"card moved on to a folder on disk", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			third := filepath.Join(t.TempDir(), "Game")
			mustMkdirFile(t, third, "game.exe", "stub")
			if _, err := r.lib.Relocate(r.game.ID, third); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: StageRepoint, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, true, false, false}} }},
		{"card moved on to a folder since deleted", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			third := filepath.Join(t.TempDir(), "Game")
			mustMkdirFile(t, third, "game.exe", "stub")
			if _, err := r.lib.Relocate(r.game.ID, third); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: StageRepoint, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target}
		}, func(t *testing.T, r rig) {
			card, err := r.lib.Find(r.game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(card.InstallDir); err != nil {
				t.Fatal(err)
			}
		}, func(rig) []fact { return []fact{{CopyTarget, false, true, false}} }},
		{"uninstalled after the repoint landed", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			if _, err := r.lib.Relocate(r.game.ID, r.target); err != nil {
				t.Fatal(err)
			}
			if err := r.lib.MarkUninstalled(r.game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageRepoint, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, false, true, false}, {CopySource, false, false, false}} }},
		{"removed at the commit, the copy in staging", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target+stagingSuffix, "game.exe", "stub")
			if err := r.lib.RemoveGame(r.game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target, Staging: r.target + stagingSuffix}
		}, func(t *testing.T, r rig) {
			if err := os.RemoveAll(r.src); err != nil {
				t.Fatal(err)
			}
		}, func(rig) []fact { return []fact{{CopyStaging, false, true, false}} }},
		{"removed at the commit, a folder at the target beside the staging", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target+stagingSuffix, "game.exe", "stub")
			mustMkdirFile(t, r.target, "game.exe", "stub")
			if err := r.lib.RemoveGame(r.game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target, Staging: r.target + stagingSuffix}
		}, nil, func(rig) []fact { return []fact{{CopyStaging, false, false, false}, {CopyTarget, false, false, false}} }},
		{"removed at the commit after its rename landed", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			if err := r.lib.RemoveGame(r.game.ID); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target, Staging: r.target + stagingSuffix}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, false, true, false}} }},
		{"undecided commit, card at the old folder that is gone", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			mustMkdirFile(t, r.target+stagingSuffix, "game.exe", "stub")
			if err := os.RemoveAll(r.src); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: ErrAmbiguousRecovery.Error(), GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target, Staging: r.target + stagingSuffix}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, false, false, false}} }},
		{"undecided commit, card at the old folder", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			mustMkdirFile(t, r.target+stagingSuffix, "game.exe", "stub")
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: ErrAmbiguousRecovery.Error(), GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target, Staging: r.target + stagingSuffix}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, true, false, false}} }},
		{"a card in the folder around the copy", func(t *testing.T, r rig) Job {
			mustMkdirFile(t, r.target, "game.exe", "stub")
			addExeAt(t, r.lib, filepath.Join(filepath.Dir(r.target), "tool.exe"), "Tool")
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: exhausted("boom").Error(), GameID: r.game.ID, Title: "Game", Source: r.src, Target: r.target}
		}, nil, func(rig) []fact { return []fact{{CopyTarget, true, false, true}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			r := rig{lib: lib, game: game, src: src, target: filepath.Join(t.TempDir(), "Game")}
			job := tc.leave(t, r)
			s := newRecoverTestService(t, job)
			s.lib = lib
			s.recoverAll(context.Background())
			if tc.after != nil {
				tc.after(t, r)
			}

			copies, err := s.OrphanedCopies()
			if err != nil {
				t.Fatal(err)
			}
			var got []fact
			for _, c := range copies {
				got = append(got, fact{c.Kind, c.MovedOn, c.OnlyCopy, c.HoldsGame})
			}
			if want := tc.want(r); !reflect.DeepEqual(got, want) {
				t.Fatalf("copies = %+v\nfacts %+v, want %+v", copies, got, want)
			}
		})
	}
}

// A move that ended for good (out of attempts, a failed verification, an
// undecided commit) stays in the journal while any folder it may have written
// is on disk and no card names it: the staging folder, the target, and the
// old folder once the card names the target. A card whose folder only holds
// the copy does not account for it. Disk usage lists each such folder: the
// staging folder as a leftover, the others as copies the card moved on from.
// The entry goes once every one is gone or named by the game's card.
func TestFinishedMoveIsKeptWhileACopyItWroteIsOnDisk(t *testing.T) {
	type leave func(t *testing.T, lib *library.Service, game library.Game, src, target string) (Job, []copyWant, string)
	cases := []struct {
		name string
		// leave prepares the folders and the card and returns the job, the
		// copies OrphanedCopies lists and the staging folder Stagings lists.
		leave leave
	}{
		{"out of attempts at the repoint", func(t *testing.T, _ *library.Service, game library.Game, src, target string) (Job, []copyWant, string) {
			mustMkdirFile(t, target, "game.exe", "stub")
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: exhausted("boom").Error(), GameID: game.ID, Title: "Game", Source: src, Target: target},
				[]copyWant{{CopyTarget, target}}, ""
		}},
		{"a card in the folder around the copy", func(t *testing.T, lib *library.Service, game library.Game, src, target string) (Job, []copyWant, string) {
			mustMkdirFile(t, target, "game.exe", "stub")
			addExeAt(t, lib, filepath.Join(filepath.Dir(target), "tool.exe"), "Tool")
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: exhausted("boom").Error(), GameID: game.ID, Title: "Game", Source: src, Target: target},
				[]copyWant{{CopyTarget, target}}, ""
		}},
		{"verification failed at the cleanup", func(t *testing.T, lib *library.Service, game library.Game, src, target string) (Job, []copyWant, string) {
			mustMkdirFile(t, target, "game.exe", "stub")
			if _, err := lib.Relocate(game.ID, target); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: ErrVerifyFailed.Error(), GameID: game.ID, Title: "Game", Source: src, Target: target},
				[]copyWant{{CopySource, src}}, ""
		}},
		{"undecided commit", func(t *testing.T, _ *library.Service, game library.Game, src, target string) (Job, []copyWant, string) {
			mustMkdirFile(t, target, "game.exe", "stub")
			mustMkdirFile(t, target+stagingSuffix, "game.exe", "stub")
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: ErrAmbiguousRecovery.Error(), GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: target + stagingSuffix},
				[]copyWant{{CopyTarget, target}}, target + stagingSuffix
		}},
		{"copy named by the game's card", func(t *testing.T, lib *library.Service, game library.Game, src, target string) (Job, []copyWant, string) {
			mustMkdirFile(t, target, "game.exe", "stub")
			if _, err := lib.Relocate(game.ID, target); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(src); err != nil {
				t.Fatal(err)
			}
			return Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, Error: exhausted("boom").Error(), GameID: game.ID, Title: "Game", Source: src, Target: target}, nil, ""
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			job, want, staging := tc.leave(t, lib, game, src, target)
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())
			s.recoverAll(context.Background())

			if len(want) == 0 && staging == "" {
				if jobs := s.List(); len(jobs) != 0 {
					t.Fatalf("jobs = %+v, want the entry dropped: nothing it wrote is left untracked", jobs)
				}
				return
			}
			if got := s.soleJob(t); got.Stage != StageFailed || got.FailedAt != "" {
				t.Fatalf("job = %s/%q, want it kept as it ended", got.Stage, got.FailedAt)
			}
			requireCopies(t, s, job.ID, want...)
			stagings, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if staging == "" && len(stagings) != 0 || staging != "" && (len(stagings) != 1 || !samePathT(stagings[0].Path, staging)) {
				t.Fatalf("Stagings = %+v, want %q", stagings, staging)
			}

			for _, w := range want {
				if err := os.RemoveAll(w.path); err != nil {
					t.Fatal(err)
				}
			}
			if staging != "" {
				if err := os.RemoveAll(staging); err != nil {
					t.Fatal(err)
				}
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("jobs after its folders are gone = %+v, want the entry dropped", jobs)
			}
		})
	}
}
