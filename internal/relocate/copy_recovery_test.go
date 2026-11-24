package relocate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/diskreason"
)

// A move killed while copying or verifying whose old folder is gone since
// (deleted by hand, a drive that went away) may have its only bytes in the
// staging folder: recovery deletes nothing, ends the job for good, and the
// folder stays listed as possibly the only copy, not offered for deletion.
func TestRecoveryAtTheCopyKeepsTheStagingWhenTheOldFolderIsGone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stage   Stage
		removed bool
	}{
		{"copy", StageCopy, false},
		{"verify", StageVerify, false},
		{"copy, the game removed since", StageCopy, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			staging := target + stagingSuffix
			part := mustMkdirFile(t, staging, "game.exe", "the only bytes left")
			if err := os.RemoveAll(src); err != nil {
				t.Fatal(err)
			}
			if tc.removed {
				if err := lib.RemoveGame(game.ID); err != nil {
					t.Fatal(err)
				}
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, GameID: game.ID, Title: "Game", Source: src, Target: target, Staging: staging}
			s := newRecoverTestService(t, job)
			s.lib = lib

			s.recoverAll(context.Background())

			if _, err := os.Stat(part); err != nil {
				t.Fatalf("staging folder deleted with the old folder gone: %v", err)
			}
			got := s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.ambiguous_recovery") {
				t.Fatalf("job = %s/%q %q, want it failed for good by relocate.ambiguous_recovery", got.Stage, got.FailedAt, got.Error)
			}
			stagings, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if len(stagings) != 1 || stagings[0].Deletable || stagings[0].Code != diskreason.OnlyCopy {
				t.Fatalf("Stagings = %+v, want the folder listed as possibly the only copy", stagings)
			}
		})
	}
}
