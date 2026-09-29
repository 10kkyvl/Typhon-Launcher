package savebackup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

// Windows refuses to rename a directory while a file inside it is open, which
// is how an antivirus scan or an open Explorer window shows up in practice.

func TestRotationFailureKeepsTheNewCopyAndReportsTheError(t *testing.T) {
	h := newHarness(t)
	h.setLimit(1)
	old := snapOf(t, h, KindUpdate)
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
	h.drain()

	held, err := os.Open(filepath.Join(old.Path, "slot1.sav"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindUpdate)
	if closeErr := held.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	if err == nil || uierr.Code(err) != codeRotationFailed {
		t.Fatalf("SnapshotPath = %v, want a rotation error", err)
	}
	if snap.ID == "" || !exists(t, snap.Path) {
		t.Fatalf("snapshot = %+v, the copy must exist and be returned with the error", snap)
	}
	ev := h.next()
	if ev.Status != StatusCreated || ev.Code != codeRotationFailed || ev.Error == "" {
		t.Fatalf("event = %+v, want created with the rotation error", ev)
	}
	if !exists(t, old.Path) {
		t.Fatal("the old copy disappeared although it could not be deleted")
	}

	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "again")
	if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindUpdate); err != nil {
		t.Fatalf("SnapshotPath once the lock is gone: %v", err)
	}
	if got := len(h.list()); got != 1 {
		t.Fatalf("%d snapshots after the retry, want the limit of 1", got)
	}
}

func TestRestoreRollsBackWhenTheCurrentSavesCannotBeMovedAside(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "played further")
	h.drain()

	held, err := os.Open(filepath.Join(h.saves, "slot1.sav"))
	if err != nil {
		t.Fatal(err)
	}
	restoreErr := h.svc.Restore(context.Background(), gameID, snap.ID)
	if closeErr := held.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	if restoreErr == nil {
		t.Fatal("restore succeeded although the saves folder is locked")
	}
	if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "played further" {
		t.Fatalf("saves = %q, a failed restore must leave them as they were", got)
	}
	for _, leftover := range []string{h.saves + stagingSuffix, h.saves + previousSuffix, h.svc.journalPath(gameID)} {
		if exists(t, leftover) {
			t.Fatalf("%s left after a rolled-back restore", leftover)
		}
	}

	if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
		t.Fatalf("restore once the lock is gone: %v", err)
	}
	sameTree(t, tree(t, h.saves), original())
}
