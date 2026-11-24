package relocate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/settings"
)

// refusedCommit is the start after a move of Game killed at its commit, with
// a Tool card added inside the copy's staging folder since: recovery left the
// job failed at the commit, to be resumed once the Tool is gone.
type refusedCommit struct {
	s       *Service
	lib     *library.Service
	dir     string
	game    library.Game
	src     string
	target  string
	staging string
	tool    library.Game
	old     Job
}

func newRefusedCommit(t *testing.T, set *settings.Service, lib *library.Service, gamesDir string) refusedCommit {
	t.Helper()
	game := addGameAt(t, lib, gamesDir, "Game")
	target := filepath.Join(t.TempDir(), "Game")
	staging := target + stagingSuffix
	mustMkdirFile(t, staging, "game.exe", "stub")
	tool := addExeAt(t, lib, filepath.Join(staging, "Tool", "tool.exe"), "Tool")
	old := Job{ID: "old", Scope: ScopeGame, Stage: StageCommit, GameID: game.ID, Title: "Game", Source: game.InstallDir, Target: target, Staging: staging}
	dir := t.TempDir()
	s := serviceWithJournalAt(t, dir, set, lib, old)
	got, err := s.jobSnapshot(old.ID)
	if err != nil || got.Stage != StageFailed || got.FailedAt != StageCommit {
		t.Fatalf("old job after the start = %+v, %v; want it failed at the commit, to be resumed", got, err)
	}
	return refusedCommit{s: s, lib: lib, dir: dir, game: game, src: game.InstallDir, target: target, staging: staging, tool: tool, old: got}
}

// dropTool takes the card in the way out of the library and its files off
// the staging folder, as the player would.
func (r refusedCommit) dropTool(t *testing.T) {
	t.Helper()
	if err := r.lib.RemoveGame(r.tool.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(r.staging, "Tool")); err != nil {
		t.Fatal(err)
	}
}

func requireMovedOn(t *testing.T, job Job) {
	t.Helper()
	if job.Stage != StageFailed || job.FailedAt != "" || !job.Orphaned || !strings.Contains(job.Error, "relocate.moved_on") {
		t.Fatalf("job = %s/%q orphaned=%v error=%q, want it over by relocate.moved_on", job.Stage, job.FailedAt, job.Orphaned, job.Error)
	}
}

type copyWant struct {
	kind, path string
}

func requireCopies(t *testing.T, s *Service, jobID string, want ...copyWant) {
	t.Helper()
	copies, err := s.OrphanedCopies()
	if err != nil {
		t.Fatal(err)
	}
	var mine []OrphanedCopy
	for _, c := range copies {
		if c.JobID == jobID {
			mine = append(mine, c)
		}
	}
	if len(mine) != len(want) {
		t.Fatalf("OrphanedCopies of %s = %+v, want %+v", jobID, mine, want)
	}
	for i, w := range want {
		if mine[i].Kind != w.kind || !samePathT(mine[i].Path, w.path) || !mine[i].MovedOn {
			t.Fatalf("OrphanedCopies[%d] = %+v, want the %s at %s marked as one the card moved on from", i, mine[i], w.kind, w.path)
		}
	}
}

// The player moved the game again while the earlier move waited for a card
// to leave its copy; the next start must not repoint the card back to that
// earlier, stale copy, and the copy stays listed.
func TestRecoveryNeverRepointsACardThatMovedOn(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	r := newRefusedCommit(t, nil, lib, t.TempDir())

	parent2 := t.TempDir()
	job, err := r.s.MoveGame(r.game.ID, parent2)
	if err != nil {
		t.Fatalf("new move of the same game = %v", err)
	}
	if done := r.s.wait(job.ID); done.Stage != "" && done.Stage != StageDone {
		t.Fatalf("new move = %s (%s)", done.Stage, done.Error)
	}
	newDir := filepath.Join(parent2, "Game")
	mustMkdirFile(t, newDir, "save.dat", "progress made after the new move")
	r.dropTool(t)

	r.s.recoverAll(context.Background())

	if card, err := r.lib.Find(r.game.ID); err != nil || !samePathT(card.InstallDir, newDir) {
		t.Fatalf("card after the next start at %s (%v); want it at %s, the newest copy", card.InstallDir, err, newDir)
	}
	old, err := r.s.jobSnapshot(r.old.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireMovedOn(t, old)
	if _, err := os.Stat(filepath.Join(r.staging, "game.exe")); err != nil {
		t.Fatalf("the earlier move's copy: %v", err)
	}
	if _, err := os.Stat(r.target); !os.IsNotExist(err) {
		t.Fatalf("the earlier move's copy was committed: %v", err)
	}
	requireCopies(t, r.s, r.old.ID, copyWant{CopyStaging, r.staging})
}

// The card came to name a third folder (an install or a scan rebound it, or
// the journal predates the rule that a new move ends the old one): recovery
// repoints nothing, deletes nothing, ends the job and lists every copy it
// has on disk. Past the repoint the card must name the target: one that
// names the old folder again moved on too, and that folder is its own.
func TestRecoveryEndsAMoveWhoseCardNamesAnotherFolder(t *testing.T) {
	cases := []struct {
		name     string
		stage    Stage
		failedAt Stage
		staged   bool
		// cardAtSource: the card names the old folder, not a third one.
		cardAtSource bool
		want         func(src, target, staging string) []copyWant
	}{
		{"killed at the commit", StageCommit, "", true, false, func(_, _, staging string) []copyWant {
			return []copyWant{{CopyStaging, staging}}
		}},
		{"failed at the repoint", StageFailed, StageRepoint, false, false, func(_, target, _ string) []copyWant {
			return []copyWant{{CopyTarget, target}}
		}},
		{"killed at the cleanup", StageCleanup, "", false, false, func(src, target, _ string) []copyWant {
			return []copyWant{{CopyTarget, target}, {CopySource, src}}
		}},
		{"killed at the cleanup, card back at the old folder", StageCleanup, "", false, true, func(_, target, _ string) []copyWant {
			return []copyWant{{CopyTarget, target}}
		}},
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
			staging := ""
			kept := []string{filepath.Join(src, "game.exe")}
			if tc.staged {
				staging = target + stagingSuffix
				kept = append(kept, mustMkdirFile(t, staging, "game.exe", "stub"))
			} else {
				kept = append(kept, mustMkdirFile(t, target, "game.exe", "stub"))
			}
			cardAt := src
			if !tc.cardAtSource {
				cardAt = filepath.Join(t.TempDir(), "Elsewhere")
				mustMkdirFile(t, cardAt, "game.exe", "stub")
				if _, err := lib.Relocate(game.ID, cardAt); err != nil {
					t.Fatal(err)
				}
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, FailedAt: tc.failedAt, GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}

			s.recoverAll(context.Background())

			requireMovedOn(t, s.soleJob(t))
			if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, cardAt) {
				t.Fatalf("card = %+v, %v; want it left at %s", card, err, cardAt)
			}
			for _, path := range kept {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("recovery touched a folder of a move the card moved on from: %v", err)
				}
			}
			want := tc.want(src, target, staging)
			requireCopies(t, s, job.ID, want...)
			stagings, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if len(stagings) != 0 {
				t.Fatalf("Stagings = %+v, want the copies listed only by OrphanedCopies", stagings)
			}

			for _, w := range want {
				if err := os.RemoveAll(w.path); err != nil {
					t.Fatal(err)
				}
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("jobs after the copies are gone = %+v, want the entry dropped", jobs)
			}
		})
	}
}

// A new move of a game whose earlier move waits to be resumed ends that move
// in the same journal write that registers it: there is never a journal with
// both resumable, which a kill would hand to the next start. A write that
// fails changes neither.
func TestNewMoveEndsTheResumableMoveOfTheGameInTheSameWrite(t *testing.T) {
	t.Run("journaled", func(t *testing.T) {
		forceCopyPath(t)
		lib := newTestLibrary(t)
		r := newRefusedCommit(t, nil, lib, t.TempDir())

		job, err := r.s.MoveGame(r.game.ID, t.TempDir())
		if err != nil {
			t.Fatalf("MoveGame = %v", err)
		}
		onDisk, err := newStore(r.dir).loadJournal()
		if err != nil {
			t.Fatal(err)
		}
		r.s.wait(job.ID)
		found := false
		for _, j := range onDisk {
			if j.ID == r.old.ID {
				found = true
				requireMovedOn(t, j)
			}
		}
		if !found {
			t.Fatalf("journal on disk = %+v, want the earlier move kept and ended", onDisk)
		}
	})
	t.Run("not journaled", func(t *testing.T) {
		forceCopyPath(t)
		lib := newTestLibrary(t)
		r := newRefusedCommit(t, nil, lib, t.TempDir())
		blockJournalDir(t, r.s, t.TempDir())

		if _, err := r.s.MoveGame(r.game.ID, t.TempDir()); err == nil {
			t.Fatal("MoveGame with a journal that cannot be written started")
		}
		if jobs := r.s.List(); len(jobs) != 1 || jobs[0].Orphaned || jobs[0].FailedAt != StageCommit {
			t.Fatalf("jobs = %+v, want only the earlier move, still resumable", jobs)
		}
		onDisk, err := newStore(r.dir).loadJournal()
		if err != nil {
			t.Fatal(err)
		}
		if len(onDisk) != 1 || onDisk[0].Orphaned || onDisk[0].FailedAt != StageCommit {
			t.Fatalf("journal on disk = %+v, want only the earlier move, still resumable", onDisk)
		}
		if card, err := lib.Find(r.game.ID); err != nil || !samePathT(card.InstallDir, r.src) {
			t.Fatalf("card = %+v, %v; want it where it was", card, err)
		}
	})
}

// A library move is a new move of every game it takes: starting a game's
// item ends that game's resumable move as MoveGame does.
func TestLibraryMoveEndsTheResumableMoveOfAGameItMoves(t *testing.T) {
	set := newTestSettings(t)
	lib := newTestLibrary(t)
	r := newRefusedCommit(t, set, lib, set.GetSettings().GamesPath)
	if err := lib.RemoveGame(r.tool.ID); err != nil {
		t.Fatal(err)
	}
	external := library.ExternalGame{Provider: library.ProviderSteam, AppID: "42", Title: "Tool", InstallDir: filepath.Join(r.staging, "Tool"), Executable: filepath.Join(r.staging, "Tool", "tool.exe")}
	if _, err := lib.ApplyExternal(t.Context(), []library.ExternalGame{external}); err != nil {
		t.Fatal(err)
	}

	job, err := r.s.MoveLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("MoveLibrary = %v", err)
	}
	if done := r.s.wait(job.ID); done.Stage != "" && done.Stage != StageDone {
		t.Fatalf("library move = %s (%s)", done.Stage, done.Error)
	}

	old, err := r.s.jobSnapshot(r.old.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireMovedOn(t, old)
	if _, err := os.Stat(filepath.Join(r.staging, "game.exe")); err != nil {
		t.Fatalf("the earlier move's copy: %v", err)
	}
	for _, g := range lib.GetGames() {
		if g.External != nil {
			if err := lib.RemoveGame(g.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	requireCopies(t, r.s, r.old.ID, copyWant{CopyStaging, r.staging})
}

// Recovery acts on the copy of the journal it took at the start. A new move
// that ended the job since (and then failed, leaving the card where it was)
// makes that copy stale: the job is read again under the reservation and not
// resumed.
func TestRecoveryLeavesAJobANewMoveEndedSinceItsSnapshot(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	r := newRefusedCommit(t, nil, lib, t.TempDir())
	afterCopyBeforeVerify = func(staging string) {
		if err := os.WriteFile(filepath.Join(staging, "extra.bin"), []byte("x"), 0o600); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterCopyBeforeVerify = nil })
	job, err := r.s.MoveGame(r.game.ID, t.TempDir())
	if err != nil {
		t.Fatalf("MoveGame = %v", err)
	}
	if done := r.s.wait(job.ID); done.Stage != StageFailed {
		t.Fatalf("new move = %s (%s), want it failed by its verification", done.Stage, done.Error)
	}
	r.dropTool(t)

	r.s.recoverJob(context.Background(), r.old)

	if card, err := lib.Find(r.game.ID); err != nil || !samePathT(card.InstallDir, r.src) {
		t.Fatalf("card = %+v, %v; want it left at %s", card, err, r.src)
	}
	if _, err := os.Stat(filepath.Join(r.src, "game.exe")); err != nil {
		t.Fatalf("the card's folder: %v", err)
	}
	if _, err := os.Stat(r.target); !os.IsNotExist(err) {
		t.Fatalf("the ended move's copy was committed: %v", err)
	}
	old, err := r.s.jobSnapshot(r.old.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireMovedOn(t, old)
}

// A move that renamed its folder took the old folder away: a folder at the
// old place at the next start is not the move's leftover (the game was
// installed there again, with a new save). Recovery never deletes it; while
// the card names it the move is over for that card, and its copy is listed.
func TestRenamedMoveLeavesAFolderThatAppearedAtTheOldPlace(t *testing.T) {
	cases := []struct {
		name            string
		stage, failedAt Stage
		// cardAtCopy: the repoint had landed, the card names the move's copy.
		cardAtCopy bool
	}{
		{"failed at the repoint, card at the new install", StageFailed, StageRepoint, false},
		{"killed at the repoint, card at the new install", StageRepoint, "", false},
		{"killed at the repoint, card at the copy", StageRepoint, "", true},
		{"killed at the cleanup, card at the copy", StageCleanup, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			if err := os.Rename(src, target); err != nil {
				t.Fatal(err)
			}
			if tc.cardAtCopy {
				if _, err := lib.Relocate(game.ID, target); err != nil {
					t.Fatal(err)
				}
			}
			mustMkdirFile(t, src, "game.exe", "fresh install")
			save := mustMkdirFile(t, src, "save.dat", "progress made on the fresh install")
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, FailedAt: tc.failedAt, Renamed: true, GameID: game.ID, Title: "Game", Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			if _, err := os.Stat(save); err != nil {
				t.Fatalf("the folder at the old place was deleted: %v; jobs %+v", err, s.List())
			}
			if _, err := os.Stat(filepath.Join(target, "game.exe")); err != nil {
				t.Fatalf("the move's copy: %v", err)
			}
			card, err := lib.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.cardAtCopy {
				if !samePathT(card.InstallDir, target) {
					t.Fatalf("card at %s, want it left at the copy %s", card.InstallDir, target)
				}
				if jobs := s.List(); len(jobs) != 0 {
					t.Fatalf("jobs = %+v, want the move finished: its card names its copy", jobs)
				}
				return
			}
			if !samePathT(card.InstallDir, src) {
				t.Fatalf("card repointed to %s, the move's copy; want it at the new install %s", card.InstallDir, src)
			}
			requireMovedOn(t, s.soleJob(t))
			requireCopies(t, s, job.ID, copyWant{CopyTarget, target})
		})
	}
}

// A renamed move has no old folder of its own once the rename landed: a
// folder at the old place is never listed as one of its copies, also after
// the card that named it left.
func TestRenamedMoveNeverListsTheOldPlaceAsItsCopy(t *testing.T) {
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "Game")
	if err := os.Rename(src, target); err != nil {
		t.Fatal(err)
	}
	mustMkdirFile(t, src, "game.exe", "fresh install")
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, Renamed: true, GameID: game.ID, Title: "Game", Source: src, Target: target}
	s := newRecoverTestService(t, job)
	s.lib = lib
	s.recoverAll(context.Background())
	requireMovedOn(t, s.soleJob(t))
	if err := lib.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}

	copies, err := s.OrphanedCopies()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range copies {
		if samePathT(c.Path, src) {
			t.Fatalf("OrphanedCopies = %+v, lists the folder at the old place as the move's", copies)
		}
	}
}

// The same for a library move's screenshots folder: one that exists again at
// the old place holds screenshots taken since, and is never deleted.
func TestRenamedLibraryItemKeepsAFolderThatAppearedAtTheOldPlace(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "old", "Screenshots")
	target := filepath.Join(root, "new", "Screenshots")
	mustMkdirFile(t, target, "shot1.png", "moved")
	shot := mustMkdirFile(t, src, "shot2.png", "taken after the failure")
	job := Job{ID: "j1", Scope: ScopeLibrary, Stage: StageRepoint, Renamed: true, GameID: itemScreenshots, Title: "Скриншоты", Source: src, Target: target}
	s := newRecoverTestService(t, job)

	s.recoverAll(context.Background())

	if _, err := os.Stat(shot); err != nil {
		t.Fatalf("screenshots taken at the old place deleted: %v; jobs %+v", err, s.List())
	}
	got := s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.source_changed") {
		t.Fatalf("job = %s/%q %q, want it failed for good by relocate.source_changed", got.Stage, got.FailedAt, got.Error)
	}
}

// On the copy path the old folder is deleted at a resumed cleanup only while
// it holds nothing the verified copy lacks: a file added or changed since the
// manifest keeps the folder, the job fails for good by relocate.source_changed
// and the folder is listed as the move's old folder. A folder the failed
// cleanup already emptied in part is still finished off.
func TestResumedCleanupKeepsAnOldFolderThatChanged(t *testing.T) {
	cases := []struct {
		name            string
		stage, failedAt Stage
		change          func(t *testing.T, src string)
		wantKept        bool
	}{
		{"killed, a save added", StageCleanup, "", func(t *testing.T, src string) {
			mustMkdirFile(t, src, "save.dat", "progress")
		}, true},
		{"failed, a file changed", StageFailed, StageCleanup, func(t *testing.T, src string) {
			mustMkdirFile(t, src, "game.exe", "patched")
		}, true},
		{"failed, a file already deleted", StageFailed, StageCleanup, func(t *testing.T, src string) {
			if err := os.Remove(filepath.Join(src, "game.exe")); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			mustMkdirFile(t, src, "data.bin", "payload")
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "Game")
			mustMkdirFile(t, target, "game.exe", "stub")
			mustMkdirFile(t, target, "data.bin", "payload")
			if _, err := lib.Relocate(game.ID, target); err != nil {
				t.Fatal(err)
			}
			tc.change(t, src)
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, FailedAt: tc.failedAt, GameID: game.ID, Title: "Game", Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}

			s.recoverAll(context.Background())

			if !tc.wantKept {
				if exists(src) {
					t.Fatalf("old folder kept, want the cleanup finished; jobs %+v", s.List())
				}
				if jobs := s.List(); len(jobs) != 0 {
					t.Fatalf("jobs = %+v, want the move finished", jobs)
				}
				return
			}
			if _, err := os.Stat(filepath.Join(src, "data.bin")); err != nil {
				t.Fatalf("old folder deleted although it changed since the manifest: %v; jobs %+v", err, s.List())
			}
			got := s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.source_changed") {
				t.Fatalf("job = %s/%q %q, want it failed for good by relocate.source_changed", got.Stage, got.FailedAt, got.Error)
			}
			copies, err := s.OrphanedCopies()
			if err != nil {
				t.Fatal(err)
			}
			if len(copies) != 1 || copies[0].Kind != CopySource || !samePathT(copies[0].Path, src) {
				t.Fatalf("OrphanedCopies = %+v, want the old folder listed", copies)
			}
		})
	}
}
