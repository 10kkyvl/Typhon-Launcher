package savebackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/uierr"
)

func original() map[string]string {
	return map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"}
}

func TestRestoreReplacesSavesAndKeepsASafetyCopy(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "played further")
	writeFile(t, filepath.Join(h.saves, "extra.sav"), "new file")
	h.drain()

	if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	sameTree(t, tree(t, h.saves), original())
	for _, leftover := range []string{h.saves + stagingSuffix, h.saves + previousSuffix, h.svc.journalPath(gameID)} {
		if exists(t, leftover) {
			t.Fatalf("%s left after a finished restore", leftover)
		}
	}
	list := h.list()
	pre := snapshotsOfKind(list, KindPreRestore)
	if len(pre) != 1 {
		t.Fatalf("list = %+v, want one pre-restore snapshot", list)
	}
	sameTree(t, tree(t, pre[0].Path), map[string]string{
		"slot1.sav": "played further", "profile/slot2.sav": "two", "extra.sav": "new file",
	})
	events := h.drain()
	if len(events) != 2 || events[0].Status != StatusCreated || events[0].Kind != KindPreRestore ||
		events[1].Status != StatusRestored || events[1].Snapshot == nil || events[1].Snapshot.ID != snap.ID {
		t.Fatalf("events = %+v, want the pre-restore copy created and then the restore", events)
	}
}

func TestRestoreRefusesWhileTheGameRuns(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "played further")
	h.drain()
	h.games.setRunning(gameID, true)

	err := h.svc.Restore(context.Background(), gameID, snap.ID)

	if !errors.Is(err, errGameRunning) || uierr.Code(err) != "savebackup.game_running" {
		t.Fatalf("restore = %v, want %v", err, errGameRunning)
	}
	if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "played further" {
		t.Fatalf("saves = %q, a refused restore must not touch them", got)
	}
	if got := len(h.list()); got != 1 {
		t.Fatalf("%d snapshots, want no safety copy for a refused restore", got)
	}
	if exists(t, h.saves+stagingSuffix) || exists(t, h.svc.journalPath(gameID)) || len(h.drain()) != 0 {
		t.Fatal("a refused restore left staging, a journal or an event behind")
	}
}

func TestRestoreFailures(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, h *harness) string
		wantIs error
	}{
		{"unknown snapshot", func(t *testing.T, h *harness) string { return "20200101-000000-manual" }, errSnapshotNotFound},
		{"saves folder unknown", func(t *testing.T, h *harness) string {
			snap := h.create()
			h.games.setResult(gameID, library.SavesResult{})
			return snap.ID
		}, errSavesNotFound},
		{"saves folders ambiguous", func(t *testing.T, h *harness) string {
			snap := h.create()
			h.games.setResult(gameID, library.SavesResult{Candidates: []string{"a", "b"}})
			return snap.ID
		}, errSavesAmbiguous},
		{"stored path is a file", func(t *testing.T, h *harness) string {
			snap := h.create()
			file := filepath.Join(t.TempDir(), "file")
			writeFile(t, file, "x")
			h.games.setSavesDir(gameID, file)
			return snap.ID
		}, errSavesNotDir},
		{"staging leftover", func(t *testing.T, h *harness) string {
			snap := h.create()
			writeFile(t, filepath.Join(h.saves+stagingSuffix, "stale"), "x")
			return snap.ID
		}, errLeftovers},
		{"previous leftover", func(t *testing.T, h *harness) string {
			snap := h.create()
			writeFile(t, filepath.Join(h.saves+previousSuffix, "stale"), "x")
			return snap.ID
		}, errLeftovers},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			id := c.setup(t, h)
			before := tree(t, h.saves)

			err := h.svc.Restore(context.Background(), gameID, id)

			if !errors.Is(err, c.wantIs) {
				t.Fatalf("restore = %v, want %v", err, c.wantIs)
			}
			sameTree(t, tree(t, h.saves), before)
			if exists(t, h.svc.journalPath(gameID)) {
				t.Fatal("a refused restore left a journal")
			}
		})
	}
}

func TestRestoreIntoADeletedSavesFolder(t *testing.T) {
	cases := []struct {
		name string
		path []string
		wipe string
	}{
		{"parent exists", []string{"Saves"}, "Saves"},
		{"parent is gone too", []string{"Docs", "Game", "Saves"}, "Docs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			root := t.TempDir()
			dest := filepath.Join(append([]string{root}, c.path...)...)
			writeFile(t, filepath.Join(dest, "slot1.sav"), "one")
			writeFile(t, filepath.Join(dest, "profile", "slot2.sav"), "two")
			h.games.setSavesDir(gameID, dest)
			h.games.setResult(gameID, library.SavesResult{Path: dest})
			snap := h.create()
			if err := os.RemoveAll(filepath.Join(root, c.wipe)); err != nil {
				t.Fatal(err)
			}
			h.games.setResult(gameID, library.SavesResult{})

			if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
				t.Fatalf("restore into a deleted folder: %v", err)
			}

			sameTree(t, tree(t, dest), original())
			if got := len(snapshotsOfKind(h.list(), KindPreRestore)); got != 0 {
				t.Fatalf("%d pre-restore copies, want none: there was nothing to copy", got)
			}
			if exists(t, dest+stagingSuffix) || exists(t, dest+previousSuffix) || exists(t, h.svc.journalPath(gameID)) {
				t.Fatal("restore into a deleted folder left staging, previous or journal")
			}
		})
	}
}

func TestRestoreOfTheOldestSnapshotSurvivesItsOwnSafetyCopy(t *testing.T) {
	h := newHarness(t)
	h.setLimit(2)
	oldest := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "v2")
	h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "v3")
	if got := len(h.list()); got != 2 {
		t.Fatalf("%d snapshots before the restore, want 2", got)
	}

	if err := h.svc.Restore(context.Background(), gameID, oldest.ID); err != nil {
		t.Fatalf("restore of the oldest snapshot: %v", err)
	}

	sameTree(t, tree(t, h.saves), original())
	ids := map[string]bool{}
	for _, s := range h.list() {
		ids[s.ID] = true
	}
	if !ids[oldest.ID] {
		t.Fatal("the restored snapshot was rotated out by its own safety copy")
	}
}

func TestRestoreWithIdenticalSavesSkipsTheSafetyCopyButStillRestores(t *testing.T) {
	h := newHarness(t)
	snap := h.create()

	if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
		t.Fatal(err)
	}

	if got := len(snapshotsOfKind(h.list(), KindPreRestore)); got != 0 {
		t.Fatalf("%d pre-restore copies, want none: the saves equal the newest snapshot", got)
	}
	sameTree(t, tree(t, h.saves), original())
}

func TestRestoreUsesTheChosenFolderOverDetection(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	chosen := filepath.Join(t.TempDir(), "Chosen")
	writeFile(t, filepath.Join(chosen, "old.sav"), "old")
	h.games.setSavesDir(gameID, chosen)

	if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
		t.Fatal(err)
	}

	sameTree(t, tree(t, chosen), original())
	if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "one" {
		t.Fatalf("detected folder changed to %q, the restore must go to the chosen folder only", got)
	}
}

func writeJournal(t *testing.T, h *harness, dest string, hadPrevious bool) {
	t.Helper()
	data, err := json.Marshal(journal{Dest: dest, SnapshotID: "x", HadPrevious: hadPrevious, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.svc.journalPath(gameID), string(data))
}

func TestStartupRecoversEveryInterruptedRestoreState(t *testing.T) {
	cases := []struct {
		name        string
		hadPrevious bool
		build       func(t *testing.T, dest string)
		want        map[string]string
		wantErr     bool
	}{
		{
			name:        "current moved aside, new not in place: undo",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest+previousSuffix, "s.sav"), "old")
				writeFile(t, filepath.Join(dest+stagingSuffix, "s.sav"), "new")
			},
			want: map[string]string{"s.sav": "old"},
		},
		{
			name:        "undo without a staging copy",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest+previousSuffix, "s.sav"), "old")
			},
			want: map[string]string{"s.sav": "old"},
		},
		{
			name:        "swap done, previous not removed: finish",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest, "s.sav"), "new")
				writeFile(t, filepath.Join(dest+previousSuffix, "s.sav"), "old")
			},
			want: map[string]string{"s.sav": "new"},
		},
		{
			name:        "nothing moved yet: drop the staging copy",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest, "s.sav"), "old")
				writeFile(t, filepath.Join(dest+stagingSuffix, "s.sav"), "new")
			},
			want: map[string]string{"s.sav": "old"},
		},
		{
			name:        "only the journal is left",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest, "s.sav"), "new")
			},
			want: map[string]string{"s.sav": "new"},
		},
		{
			name:        "no previous ever, staging waiting: roll forward",
			hadPrevious: false,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest+stagingSuffix, "s.sav"), "new")
			},
			want: map[string]string{"s.sav": "new"},
		},
		{
			name:        "everything present: refuse to guess",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest, "s.sav"), "a")
				writeFile(t, filepath.Join(dest+stagingSuffix, "s.sav"), "b")
				writeFile(t, filepath.Join(dest+previousSuffix, "s.sav"), "c")
			},
			want:    map[string]string{"s.sav": "a"},
			wantErr: true,
		},
		{
			name:        "current existed but only staging is left: refuse to guess",
			hadPrevious: true,
			build: func(t *testing.T, dest string) {
				writeFile(t, filepath.Join(dest+stagingSuffix, "s.sav"), "b")
			},
			wantErr: true,
		},
		{
			name:        "current existed and nothing is left: refuse to guess",
			hadPrevious: true,
			build:       func(t *testing.T, dest string) {},
			wantErr:     true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.create()
			dest := filepath.Join(t.TempDir(), "Saves")
			c.build(t, dest)
			writeJournal(t, h, dest, c.hadPrevious)

			h.restart()

			if c.wantErr {
				if !exists(t, h.svc.journalPath(gameID)) {
					t.Fatal("journal removed although the state could not be resolved")
				}
				_, err := h.svc.Create(context.Background(), gameID)
				if uierr.Code(err) != "savebackup.recovery_failed" {
					t.Fatalf("Create on a game with unresolved recovery = %v, want %s", err, "savebackup.recovery_failed")
				}
				if c.want != nil {
					sameTree(t, tree(t, dest), c.want)
				}
				return
			}
			sameTree(t, tree(t, dest), c.want)
			for _, leftover := range []string{dest + stagingSuffix, dest + previousSuffix, h.svc.journalPath(gameID)} {
				if exists(t, leftover) {
					t.Fatalf("%s left after recovery", leftover)
				}
			}
			if _, err := h.svc.Create(context.Background(), gameID); err != nil {
				t.Fatalf("Create after recovery: %v", err)
			}
		})
	}
}

func TestRecoveryRefusesDamagedJournals(t *testing.T) {
	cases := []struct {
		name    string
		content func(dest string) string
	}{
		{"invalid json", func(string) string { return "{oops" }},
		{"empty", func(string) string { return "" }},
		{"relative destination", func(string) string { return `{"dest":"Saves"}` }},
		{"empty destination", func(string) string { return `{"dest":""}` }},
		{"unclean destination", func(dest string) string {
			return fmt.Sprintf(`{"dest":%q}`, dest+string(filepath.Separator)+".."+string(filepath.Separator)+"Saves")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.create()
			dest := filepath.Join(t.TempDir(), "Saves")
			writeFile(t, filepath.Join(dest+previousSuffix, "s.sav"), "precious")
			writeFile(t, h.svc.journalPath(gameID), c.content(dest))

			h.restart()

			_, err := h.svc.Create(context.Background(), gameID)
			if uierr.Code(err) != "savebackup.recovery_failed" {
				t.Fatalf("Create = %v, want a recovery failure", err)
			}
			if got := readFile(t, filepath.Join(dest+previousSuffix, "s.sav")); got != "precious" {
				t.Fatal("recovery touched the data although the journal could not be trusted")
			}
			if !exists(t, h.svc.journalPath(gameID)) {
				t.Fatal("a damaged journal was deleted instead of being reported")
			}
			other, err := h.svc.List(context.Background(), "g2")
			if err != nil || len(other) != 0 {
				t.Fatalf("another game is affected: %v %v", other, err)
			}
			h.games.setResult("g2", library.SavesResult{Path: h.saves})
			if _, err := h.svc.Create(context.Background(), "g2"); err != nil {
				t.Fatalf("a stuck game blocked another one: %v", err)
			}

			if err := os.Remove(h.svc.journalPath(gameID)); err != nil {
				t.Fatal(err)
			}
			if _, err := h.svc.Create(context.Background(), gameID); err != nil {
				t.Fatalf("Create after the journal was removed: %v", err)
			}
		})
	}
}

func TestStartupRemovesPartialSnapshots(t *testing.T) {
	h := newHarness(t)
	good := h.create()
	stale := filepath.Join(h.gameDir(), "20260101-000000-manual"+partialSuffix)
	writeFile(t, filepath.Join(stale, filesDirName, "half.sav"), "half")

	h.restart()

	if exists(t, stale) {
		t.Fatal("partial snapshot survived startup")
	}
	list := h.list()
	if len(list) != 1 || list[0].ID != good.ID {
		t.Fatalf("list = %+v, want only the finished snapshot", list)
	}
}

func TestPartialSnapshotsAreNotListedAndAreSweptBeforeNextCreate(t *testing.T) {
	h := newHarness(t)
	stale := filepath.Join(h.gameDir(), "20260101-000000-manual"+partialSuffix)
	writeFile(t, filepath.Join(stale, snapshotFile), "{}")
	if got := len(h.list()); got != 0 {
		t.Fatalf("a partial directory is listed as %d snapshots", got)
	}

	h.create()

	if exists(t, stale) {
		t.Fatal("partial snapshot survived the next create")
	}
}
