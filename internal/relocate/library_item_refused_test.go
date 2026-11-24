package relocate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"typhon/internal/history"
	"typhon/internal/library"
)

// A library move whose next game is refused before anything of it started
// fails with that game's own error and is not resumable: no stage of the
// game moved before it is left on the job, the same move can be run again in
// the session, and the next start does not finish the moved game a second
// time.
func TestLibraryItemRefusedBeforeItStartsFailsTheMoveForGood(t *testing.T) {
	for _, tc := range []struct {
		name     string
		block    func(t *testing.T, lib *library.Service, b library.Game, root string) func()
		wantCode string
	}{
		{"its target is not empty", func(t *testing.T, _ *library.Service, _ library.Game, root string) func() {
			blocker := mustMkdirFile(t, filepath.Join(root, "Games", "B"), "x.txt", "in the way")
			return func() {
				if err := os.Remove(blocker); err != nil {
					t.Fatal(err)
				}
			}
		}, "relocate.target_not_empty"},
		{"it is being removed", func(t *testing.T, lib *library.Service, b library.Game, _ string) func() {
			release, err := lib.BeginRemoval(b.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			return release
		}, "relocate.game_removing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := newTestSettings(t)
			lib := newTestLibrary(t)
			s := newTestService(t, set, lib, nil, nil, nil)
			var mu sync.Mutex
			moved := map[string]int{}
			s.SetHistoryRecorder(func(r history.Record) error {
				mu.Lock()
				defer mu.Unlock()
				moved[r.GameID]++
				return nil
			})
			games := set.GetSettings().GamesPath
			a := addGameAt(t, lib, games, "A")
			b := addGameAt(t, lib, games, "B")
			parent := t.TempDir()
			root, err := set.ProposeLibraryPath(parent)
			if err != nil {
				t.Fatal(err)
			}
			unblock := tc.block(t, lib, b, root)

			job, err := s.MoveLibrary(parent)
			if err != nil {
				t.Fatal(err)
			}
			done := s.wait(job.ID)

			if done.Stage != StageFailed || done.FailedAt != "" || !strings.Contains(done.Error, tc.wantCode) {
				t.Fatalf("library job = %s/%q %q, want failed for good by %s", done.Stage, done.FailedAt, done.Error, tc.wantCode)
			}
			if done.GameID == a.ID || done.Renamed || done.Staging != "" {
				t.Fatalf("library job still carries the moved game: %+v", done)
			}
			cardA, err := lib.Find(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !samePathT(cardA.InstallDir, filepath.Join(root, "Games", "A")) {
				t.Fatalf("A at %s, want it moved", cardA.InstallDir)
			}

			s.recoverAll(context.Background())
			mu.Lock()
			movedA := moved[a.ID]
			mu.Unlock()
			if movedA != 1 {
				t.Fatalf("A recorded as moved %d times, want once", movedA)
			}

			unblock()
			retry, err := s.MoveLibrary(parent)
			if err != nil {
				t.Fatalf("the same library move again = %v", err)
			}
			if again := s.wait(retry.ID); again.Stage != "" {
				t.Fatalf("library move again ended %s %q, want it finished", again.Stage, again.Error)
			}
			cardB, err := lib.Find(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !samePathT(cardB.InstallDir, filepath.Join(root, "Games", "B")) {
				t.Fatalf("B at %s after the retry, want it moved", cardB.InstallDir)
			}
		})
	}
}
