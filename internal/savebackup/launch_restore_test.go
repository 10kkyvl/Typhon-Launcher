package savebackup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func cloudSnapshot(t *testing.T, h *harness) Snapshot {
	t.Helper()
	downloaded := filepath.Join(t.TempDir(), "downloaded")
	writeFile(t, filepath.Join(downloaded, "slot1.sav"), "from the cloud")
	writeFile(t, filepath.Join(downloaded, "profile", "slot2.sav"), "two")
	snap, err := h.svc.SnapshotPath(context.Background(), gameID, downloaded, KindCloud)
	if err != nil {
		t.Fatalf("snapshot of the downloaded copy: %v", err)
	}
	h.drain()
	return snap
}

func wantRefused(t *testing.T, h *harness, err error) {
	t.Helper()
	if !errors.Is(err, errGameRunning) || uierr.Code(err) != "savebackup.game_running" {
		t.Fatalf("restore = %v, want %v", err, errGameRunning)
	}
	if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "one" {
		t.Fatalf("saves = %q, a refused restore must not touch them", got)
	}
	if exists(t, h.saves+stagingSuffix) || exists(t, h.svc.journalPath(gameID)) {
		t.Fatal("a refused restore left staging or a journal behind")
	}
}

// The cloud check that holds a launch restores while the library reports the
// game as starting: the launch cannot get past the check until it is done.
func TestLaunchRestoreGoesThroughWhileTheGameIsStarting(t *testing.T) {
	h := newHarness(t)
	snap := cloudSnapshot(t, h)
	h.games.setStarting(gameID, true)

	if err := h.svc.RestoreBeforeLaunch(context.Background(), gameID, snap.ID); err != nil {
		t.Fatalf("restore before launch of a starting game: %v", err)
	}

	sameTree(t, tree(t, h.saves), map[string]string{"slot1.sav": "from the cloud", "profile/slot2.sav": "two"})
	if pre := snapshotsOfKind(h.list(), KindPreRestore); len(pre) != 1 {
		t.Fatalf("%d pre-restore copies, want the replaced saves kept as one", len(pre))
	}
	if exists(t, h.saves+stagingSuffix) || exists(t, h.saves+previousSuffix) || exists(t, h.svc.journalPath(gameID)) {
		t.Fatal("the restore left staging, the previous folder or its journal behind")
	}
}

func TestLaunchRestoreRefusesAGameWhoseProcessRuns(t *testing.T) {
	h := newHarness(t)
	snap := cloudSnapshot(t, h)
	h.games.setRunning(gameID, true)

	wantRefused(t, h, h.svc.RestoreBeforeLaunch(context.Background(), gameID, snap.ID))
}

// Every restore that is not the launch check's own still waits for the game
// to be neither starting nor running.
func TestRestoreRefusesAGameThatIsStarting(t *testing.T) {
	h := newHarness(t)
	snap := cloudSnapshot(t, h)
	h.games.setStarting(gameID, true)

	wantRefused(t, h, h.svc.Restore(context.Background(), gameID, snap.ID))
}

// The game turns starting or running after the restore's first check, while
// the copy is staged: the second check, right before the rename, refuses it
// and leaves the saves and no journal behind.
func TestRestoreRefusesAGameThatStartsDuringTheCopy(t *testing.T) {
	cases := []struct {
		name    string
		restore func(s *Service, ctx context.Context, gameID, snapshotID string) error
		turn    func(f *fakeGames)
		refused bool
	}{
		{"restore, game starting", (*Service).Restore, func(f *fakeGames) { f.setStarting(gameID, true) }, true},
		{"restore, process running", (*Service).Restore, func(f *fakeGames) { f.setRunning(gameID, true) }, true},
		{"before launch, process running", (*Service).RestoreBeforeLaunch, func(f *fakeGames) { f.setRunning(gameID, true) }, true},
		{"before launch, game starting", (*Service).RestoreBeforeLaunch, func(f *fakeGames) { f.setStarting(gameID, true) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			snap := cloudSnapshot(t, h)
			h.games.setOnLocate(func() { tc.turn(h.games) })

			err := tc.restore(h.svc, context.Background(), gameID, snap.ID)

			if !tc.refused {
				if err != nil {
					t.Fatalf("restore = %v, want it to go through for a starting game", err)
				}
				sameTree(t, tree(t, h.saves), map[string]string{"slot1.sav": "from the cloud", "profile/slot2.sav": "two"})
				return
			}
			wantRefused(t, h, err)
			if exists(t, h.saves+previousSuffix) {
				t.Fatal("a refused restore left the previous folder behind")
			}
			sameTree(t, tree(t, h.saves), map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
		})
	}
}
