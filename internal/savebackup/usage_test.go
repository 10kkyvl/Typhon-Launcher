package savebackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/library"
)

func TestUsage(t *testing.T) {
	h := newHarness(t)
	a := h.create()
	b := h.create()
	h.games.setResult("g2", library.SavesResult{Path: h.saves})
	other, err := h.svc.Create(context.Background(), "g2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.dir, rootDirName, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.dir, rootDirName, "emptygame"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := h.svc.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		gameID: {GameID: gameID, Count: 2, SizeBytes: a.SizeBytes + b.SizeBytes},
		"g2":   {GameID: "g2", Count: 1, SizeBytes: other.SizeBytes},
	}
	if len(got) != len(want) {
		t.Fatalf("usage = %+v, want %d groups", got, len(want))
	}
	for _, u := range got {
		if u != want[u.GameID] {
			t.Fatalf("usage of %s = %+v, want %+v", u.GameID, u, want[u.GameID])
		}
	}
}

func TestUsageBrokenSnapshotMakesSizeUnknown(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	if err := os.Remove(filepath.Join(h.gameDir(), snap.ID, snapshotFile)); err != nil {
		t.Fatal(err)
	}

	got, err := h.svc.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Count != 1 || !got[0].SizeUnknown {
		t.Fatalf("usage = %+v, want one counted snapshot with an unknown size", got)
	}
}

func TestUsageRefusals(t *testing.T) {
	t.Run("cancelled context", func(t *testing.T) {
		h := newHarness(t)
		h.create()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := h.svc.Usage(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Usage = %v, want context.Canceled", err)
		}
	})
	t.Run("not started", func(t *testing.T) {
		h := newHarness(t)
		svc := h.build()
		if _, err := svc.Usage(context.Background()); !errors.Is(err, errNotStarted) {
			t.Fatalf("Usage = %v, want errNotStarted", err)
		}
	})
	t.Run("no snapshots folder", func(t *testing.T) {
		h := newHarness(t)
		got, err := h.svc.Usage(context.Background())
		if err != nil || len(got) != 0 {
			t.Fatalf("Usage = %v, %v; want empty", got, err)
		}
	})
	t.Run("snapshots root is a file", func(t *testing.T) {
		h := newHarness(t)
		if err := os.WriteFile(filepath.Join(h.dir, rootDirName), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.svc.Usage(context.Background()); err == nil {
			t.Fatal("an unreadable root must be an error, not an empty list")
		}
	})
}
