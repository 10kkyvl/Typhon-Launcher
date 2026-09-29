package savebackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func snapOf(t *testing.T, h *harness, kind Kind) Snapshot {
	t.Helper()
	snap, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, kind)
	if err != nil {
		t.Fatalf("snapshot of kind %s: %v", kind, err)
	}
	return snap
}

func copyingJournal(dest, snapshotID string, hadPrevious bool) string {
	return fmt.Sprintf(`{"dest":%q,"snapshotId":%q,"hadPrevious":%t,"copying":true}`, dest, snapshotID, hadPrevious)
}

func TestPreRestoreDoesNotTrustADigestOfDamagedFiles(t *testing.T) {
	h := newHarness(t)
	older := snapOf(t, h, KindUpdate)
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "live progress")
	latest := snapOf(t, h, KindUpdate)
	writeFile(t, filepath.Join(latest.Path, "slot1.sav"), "xxxxxxxxxxxxx")

	if err := h.svc.Restore(context.Background(), gameID, older.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	pre := snapshotsOfKind(h.list(), KindPreRestore)
	if len(pre) != 1 {
		t.Fatalf("%d pre-restore copies, want 1: the newest snapshot has the right digest but damaged files, so it is not a safety copy", len(pre))
	}
	sameTree(t, tree(t, pre[0].Path), map[string]string{"slot1.sav": "live progress", "profile/slot2.sav": "two"})
}

func TestMigrationDoesNotTrustADigestOfDamagedFiles(t *testing.T) {
	h := newHarness(t)
	existing := snapOf(t, h, KindUpdate)
	writeFile(t, filepath.Join(existing.Path, "slot1.sav"), "xyz")
	legacy := filepath.Join(h.dir, legacyDirName, gameID)
	writeFile(t, filepath.Join(legacy, "slot1.sav"), "one")
	writeFile(t, filepath.Join(legacy, "profile", "slot2.sav"), "two")

	h.restart()

	intact := 0
	for _, s := range snapshotsOfKind(h.list(), KindUpdate) {
		got := tree(t, s.Path)
		if got["slot1.sav"] == "one" && got["profile/slot2.sav"] == "two" {
			intact++
		}
	}
	if intact != 1 {
		t.Fatalf("%d intact update snapshots after the migration, want 1: the legacy folder was deleted against a snapshot whose files are damaged", intact)
	}
	if exists(t, legacy) {
		t.Fatal("legacy directory left after a successful migration")
	}
}

func TestRestoreRefusesADamagedSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(t *testing.T, files string)
	}{
		{"same size, other content", func(t *testing.T, files string) { writeFile(t, filepath.Join(files, "slot1.sav"), "xyz") }},
		{"file deleted", func(t *testing.T, files string) {
			if err := os.Remove(filepath.Join(files, "slot1.sav")); err != nil {
				t.Fatal(err)
			}
		}},
		{"extra file", func(t *testing.T, files string) { writeFile(t, filepath.Join(files, "extra.sav"), "extra") }},
		{"file grown", func(t *testing.T, files string) { writeFile(t, filepath.Join(files, "slot1.sav"), "one and more") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			snap := h.create()
			writeFile(t, filepath.Join(h.saves, "slot1.sav"), "played further")
			c.tamper(t, snap.Path)
			h.drain()

			err := h.svc.Restore(context.Background(), gameID, snap.ID)

			if !errors.Is(err, errSnapshotBroken) || uierr.Code(err) != "savebackup.snapshot_broken" {
				t.Fatalf("restore = %v, want %v", err, errSnapshotBroken)
			}
			if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "played further" {
				t.Fatalf("saves = %q, a refused restore must not touch them", got)
			}
			for _, leftover := range []string{h.saves + stagingSuffix, h.saves + previousSuffix, h.svc.journalPath(gameID)} {
				if exists(t, leftover) {
					t.Fatalf("%s left after a refused restore", leftover)
				}
			}
			if got := len(snapshotsOfKind(h.list(), KindPreRestore)); got != 0 {
				t.Fatalf("%d pre-restore copies for a restore that was refused", got)
			}
		})
	}
}

func TestInterruptedCopyIsDiscardedNeverRolledForward(t *testing.T) {
	cases := []struct {
		name       string
		destExists bool
	}{
		{"saves folder exists", true},
		{"saves folder was deleted", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			dest := filepath.Join(t.TempDir(), "Saves")
			writeFile(t, filepath.Join(dest, "slot1.sav"), "one")
			writeFile(t, filepath.Join(dest, "profile", "slot2.sav"), "two")
			h.games.setSavesDir(gameID, dest)
			snap := h.create()
			if c.destExists {
				writeFile(t, filepath.Join(dest, "slot1.sav"), "played further")
			} else if err := os.RemoveAll(dest); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest+stagingSuffix, "slot1.sav"), "half a fi")
			writeFile(t, h.svc.journalPath(gameID), copyingJournal(dest, snap.ID, c.destExists))

			h.restart()

			if exists(t, dest+stagingSuffix) || exists(t, h.svc.journalPath(gameID)) {
				t.Fatal("staging or journal left after recovery of an interrupted copy")
			}
			if !c.destExists && exists(t, dest) {
				t.Fatalf("recovery rolled a half-copied staging folder forward into %s", dest)
			}
			if c.destExists {
				if got := readFile(t, filepath.Join(dest, "slot1.sav")); got != "played further" {
					t.Fatalf("saves = %q, recovery must not touch them", got)
				}
			}
			if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
				t.Fatalf("restore after an interrupted one: %v", err)
			}
			sameTree(t, tree(t, dest), original())
		})
	}
}

func TestInterruptedCopyWithAPreviousFolderIsRefused(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	writeFile(t, filepath.Join(h.saves+previousSuffix, "slot1.sav"), "precious")
	writeFile(t, filepath.Join(h.saves+stagingSuffix, "slot1.sav"), "half")
	writeFile(t, h.svc.journalPath(gameID), copyingJournal(h.saves, snap.ID, true))

	h.restart()

	if got := readFile(t, filepath.Join(h.saves+previousSuffix, "slot1.sav")); got != "precious" {
		t.Fatal("recovery touched a previous folder it had no business with")
	}
	if _, err := h.svc.Create(context.Background(), gameID); uierr.Code(err) != "savebackup.recovery_failed" {
		t.Fatalf("Create = %v, want a recovery failure", err)
	}
}

func TestManualCopiesAreOutsideTheRotationLimit(t *testing.T) {
	h := newHarness(t)
	h.setLimit(2)
	manual := h.create()
	for i := 0; i < 3; i++ {
		writeFile(t, filepath.Join(h.saves, "slot1.sav"), string(rune('a'+i)))
		snapOf(t, h, KindSession)
	}

	list := h.list()
	if got := len(snapshotsOfKind(list, KindSession)); got != 2 {
		t.Fatalf("%d session copies, want the limit of 2", got)
	}
	if got := snapshotsOfKind(list, KindManual); len(got) != 1 || got[0].ID != manual.ID {
		t.Fatalf("manual copies = %+v, want the one the player made", got)
	}

	h.setLimit(1)
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "later")
	snapOf(t, h, KindSession)
	list = h.list()
	if got := len(snapshotsOfKind(list, KindSession)); got != 1 {
		t.Fatalf("%d session copies after lowering the limit, want 1", got)
	}
	if got := len(snapshotsOfKind(list, KindManual)); got != 1 {
		t.Fatalf("%d manual copies after lowering the limit, want 1", got)
	}
}

func TestSnapshotIDsHaveOneStrictFormat(t *testing.T) {
	valid := []string{
		"20260929-153001-manual", "20260929-153001-session", "20260929-153001-update",
		"20260929-153001-pre-restore", "20260929-153001-manual-2", "20260929-153001-pre-restore-17",
	}
	invalid := []string{
		"restore.json", "snapshot.json", "files", "g1", "20260929-153001", "20260929-153001-other",
		"2026092-153001-manual", "20260929-15300-manual", "20260929-153001-manual.partial", "20260929-153001-manual-",
		"20260929-153001-manual-x", "x20260929-153001-manual", "20260929-153001-manual ", "..", "",
		"20260929-153001-manual/..",
	}
	for _, id := range valid {
		if !validSnapshotID(id) {
			t.Errorf("validSnapshotID(%q) = false, want true", id)
		}
	}
	for _, id := range invalid {
		if validSnapshotID(id) {
			t.Errorf("validSnapshotID(%q) = true, want false", id)
		}
	}
}

func TestDeleteNeverTouchesTheJournalOrPlainFiles(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	writeFile(t, h.svc.journalPath(gameID), `{"dest":"x"}`)
	stray := filepath.Join(h.gameDir(), "20260101-000000-manual")
	writeFile(t, stray, "a plain file that only looks like a snapshot")

	if err := h.svc.Delete(gameID, "restore.json"); !errors.Is(err, errInvalidID) {
		t.Fatalf("Delete(restore.json) = %v, want %v", err, errInvalidID)
	}
	if err := h.svc.Delete(gameID, filepath.Base(stray)); !errors.Is(err, errSnapshotNotFound) {
		t.Fatalf("Delete(plain file) = %v, want %v", err, errSnapshotNotFound)
	}
	if !exists(t, h.svc.journalPath(gameID)) || !exists(t, stray) || !exists(t, snap.Path) {
		t.Fatal("Delete removed something that is not a snapshot directory")
	}
}

func TestDeleteRefusesASymlinkNamedLikeASnapshot(t *testing.T) {
	h := newHarness(t)
	victim := filepath.Join(t.TempDir(), "victim")
	writeFile(t, filepath.Join(victim, "keep.txt"), "x")
	link := filepath.Join(h.gameDir(), "20260101-000000-manual")
	if err := os.MkdirAll(h.gameDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("this machine cannot create symlinks: %v", err)
	}

	if err := h.svc.Delete(gameID, filepath.Base(link)); !errors.Is(err, errSnapshotNotFound) {
		t.Fatalf("Delete(symlink) = %v, want %v", err, errSnapshotNotFound)
	}
	if !exists(t, filepath.Join(victim, "keep.txt")) {
		t.Fatal("Delete followed a symlink out of the backup directory")
	}
}

func TestStartupDoesNotWaitForRecoveryOfAGame(t *testing.T) {
	dir := t.TempDir()
	games := &fakeGames{games: map[string]library.Game{gameID: {ID: gameID}}, results: map[string]library.SavesResult{}, running: map[string]bool{}}
	svc, err := newServiceAt(dir, settings.Defaults, games)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, legacyDirName, gameID, "a.sav"), "legacy")
	unlock := svc.lockGame(gameID)
	released := false
	release := func() {
		if !released {
			released = true
			unlock()
		}
	}
	defer release()

	started := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { started <- svc.ServiceStartup(ctx, application.ServiceOptions{}) }()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceStartup waited for a game that is still locked: migration must not block the launcher start")
	}

	release()
	svc.wg.Wait()
	list, err := svc.list(context.Background(), gameID)
	if err != nil || len(list) != 1 || list[0].Kind != KindUpdate {
		t.Fatalf("list = %+v, %v, want the migrated legacy snapshot", list, err)
	}
	cancel()
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}
