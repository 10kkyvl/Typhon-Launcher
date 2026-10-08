package savebackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHiddenBackupsRefuseEveryPlayerCall(t *testing.T) {
	h := newHarness(t)
	existing := h.create()
	h.drain()
	h.svc.enabled = false

	if h.svc.Enabled() {
		t.Fatal("Enabled() = true for a hidden service")
	}
	calls := []struct {
		name string
		call func() error
	}{
		{"list", func() error { _, err := h.svc.List(context.Background(), gameID); return err }},
		{"create", func() error { _, err := h.svc.Create(context.Background(), gameID); return err }},
		{"restore", func() error { return h.svc.Restore(context.Background(), gameID, existing.ID) }},
		{"delete", func() error { return h.svc.Delete(gameID, existing.ID) }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(); !errors.Is(err, errDisabled) {
				t.Fatalf("%s = %v, want %v", c.name, err, errDisabled)
			}
		})
	}

	entries, err := os.ReadDir(h.gameDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != existing.ID {
		t.Fatalf("backup folder = %v, want only %s untouched", entries, existing.ID)
	}
	if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "one" {
		t.Fatalf("saves changed to %q by a hidden restore", got)
	}
}

func TestHiddenBackupsTakeNoCopyAfterASession(t *testing.T) {
	h := newHarness(t)
	h.svc.enabled = false

	h.svc.SessionStopped(gameID)
	h.svc.wg.Wait()

	if events := h.drain(); len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
	if _, err := os.Stat(h.gameDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat backup folder = %v, want it absent", err)
	}
}

func TestHiddenBackupsKeepThePreUpdateSnapshot(t *testing.T) {
	h := newHarness(t)
	h.svc.enabled = false

	snap, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindUpdate)
	if err != nil {
		t.Fatalf("pre-update snapshot: %v", err)
	}
	if snap.Kind != KindUpdate || snap.Files != 2 {
		t.Fatalf("snapshot = %+v, want the update copy of both files", snap)
	}
}
