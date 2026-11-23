package relocate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// serviceWithJournal is the next start of a launcher that left jobs in its
// journal, with the startup recovery already run.
func serviceWithJournal(t *testing.T, lib *library.Service, jobs ...Job) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := newStore(dir).saveJournal(jobs); err != nil {
		t.Fatal(err)
	}
	s, err := NewServiceAt(dir, nil, lib, nil, nil, nil)
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
	s.wg.Wait()
	return s
}

// A move that failed or was cancelled while copying and could not delete its
// staging folder stays in the journal, as a finished job, for as long as that
// folder is on disk: Disk usage lists it as a leftover the player may delete.
// Once the folder is gone the entry goes too.
func TestFailedMoveStaysListedWhileItsStagingFolderIsOnDisk(t *testing.T) {
	for _, stage := range []Stage{StageFailed, StageCancelled} {
		t.Run(string(stage), func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			staging := target + stagingSuffix
			mustMkdirFile(t, staging, "half.bin", "half a copy")
			job := Job{ID: "j1", Scope: ScopeGame, Stage: stage, Error: "boom", GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			got := s.soleJob(t)
			if got.Stage != stage {
				t.Fatalf("job = %s, want it kept as %s", got.Stage, stage)
			}
			stagings, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if len(stagings) != 1 || !samePathT(stagings[0].Path, staging) || !stagings[0].Deletable {
				t.Fatalf("Stagings = %+v, want the staging folder listed as deletable", stagings)
			}

			if err := os.RemoveAll(staging); err != nil {
				t.Fatal(err)
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("jobs after the staging folder is gone = %+v, want the entry dropped", jobs)
			}
		})
	}
}

// The player retries the same move: it takes over the staging folder its own
// earlier attempt left, deletes it and copies again.
func TestRetryOfAMoveTakesOverItsOwnLeftStagingFolder(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	parent := t.TempDir()
	target := filepath.Join(parent, "Game")
	staging := target + stagingSuffix
	leftover := mustMkdirFile(t, staging, "half.bin", "half a copy")
	old := Job{ID: "old", Scope: ScopeGame, Stage: StageFailed, Error: "boom", GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
	s := serviceWithJournal(t, lib, old)

	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatalf("MoveGame = %v", err)
	}
	if done := s.wait(job.ID); done.Stage != "" && done.Stage != StageDone {
		t.Fatalf("retried move = %s (%s), want it done", done.Stage, done.Error)
	}
	if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, target) {
		t.Fatalf("card = %+v, %v; want it at %s", card, err, target)
	}
	if _, err := os.Stat(filepath.Join(target, filepath.Base(leftover))); !os.IsNotExist(err) {
		t.Fatalf("the old attempt's half copy came along into the target: %v", err)
	}
	if jobs := s.List(); len(jobs) != 0 {
		t.Fatalf("jobs = %+v, want the old attempt's entry gone with its folder", jobs)
	}
}

// A staging folder that is not this game's own left one, the leftover of a
// move of another game into the same folder or something no journaled move
// names, is neither copied into nor deleted, and the refusal says what is in
// the way: the target itself does not exist.
func TestMoveRefusesAStagingFolderLeftByAnotherMove(t *testing.T) {
	cases := []struct {
		name  string
		owner func(t *testing.T, lib *library.Service, target string) []Job
	}{
		{"no journaled move", func(*testing.T, *library.Service, string) []Job { return nil }},
		{"another game's failed move", func(t *testing.T, lib *library.Service, target string) []Job {
			other, src := addTestGame(t, lib)
			return []Job{{ID: "other", Scope: ScopeGame, Stage: StageFailed, Error: "boom", GameID: other.ID, Title: "Game", Source: src, Target: target, Staging: target + stagingSuffix}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forceCopyPath(t)
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			parent := t.TempDir()
			target := filepath.Join(parent, "Game")
			kept := mustMkdirFile(t, target+stagingSuffix, "not-this-move.bin", "someone else's")
			s := serviceWithJournal(t, lib, tc.owner(t, lib, target)...)

			job, err := s.MoveGame(game.ID, parent)
			if err != nil {
				t.Fatalf("MoveGame = %v", err)
			}
			done := s.wait(job.ID)
			if done.Stage != StageFailed || !strings.Contains(done.Error, "relocate.staging_left") {
				t.Fatalf("move = %s (%s), want it failed by relocate.staging_left", done.Stage, done.Error)
			}
			if _, err := os.Stat(kept); err != nil {
				t.Fatalf("the move deleted a staging folder that is not its own: %v", err)
			}
			if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, src) {
				t.Fatalf("card = %+v, %v; want it where it was", card, err)
			}
		})
	}
}
