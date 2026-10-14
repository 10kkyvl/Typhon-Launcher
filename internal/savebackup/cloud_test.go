package savebackup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestCloudSnapshotCanBeRestored(t *testing.T) {
	h := newHarness(t)
	downloaded := filepath.Join(t.TempDir(), "downloaded")
	writeFile(t, filepath.Join(downloaded, "slot1.sav"), "from the cloud")
	writeFile(t, filepath.Join(downloaded, "profile", "slot2.sav"), "two from the cloud")

	snap, err := h.svc.SnapshotPath(context.Background(), gameID, downloaded, KindCloud)
	if err != nil {
		t.Fatalf("snapshot of a downloaded folder: %v", err)
	}
	if snap.Kind != KindCloud || !validSnapshotID(snap.ID) {
		t.Fatalf("snapshot = %+v, want a cloud snapshot with an addressable id", snap)
	}
	if got := snapshotsOfKind(h.list(), KindCloud); len(got) != 1 || got[0].ID != snap.ID {
		t.Fatalf("list has %+v, want the cloud snapshot", got)
	}

	if err := h.svc.Restore(context.Background(), gameID, snap.ID); err != nil {
		t.Fatalf("restore of a cloud snapshot: %v", err)
	}
	sameTree(t, tree(t, h.saves), map[string]string{
		"slot1.sav": "from the cloud", "profile/slot2.sav": "two from the cloud",
	})
	if pre := snapshotsOfKind(h.list(), KindPreRestore); len(pre) != 1 {
		t.Fatalf("%d pre-restore copies, want the local saves kept as one", len(pre))
	}
}

func TestCloudKindIsRejectedOnlyWhenUnknown(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, Kind("cloudy")); !errors.Is(err, errInvalidKind) {
		t.Fatalf("err = %v, want errInvalidKind", err)
	}
}
