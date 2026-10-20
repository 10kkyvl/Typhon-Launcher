package screenshots

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsage(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	r := newRig(t, fakeGames{})
	writeFile(t, filepath.Join(r.root, "g1", "a.png"), at)
	writeFile(t, filepath.Join(r.root, "g1", "b.png"), at)
	writeFile(t, filepath.Join(r.root, "g1", "notes.txt"), at)
	writeFile(t, filepath.Join(r.root, "g2", "c.png"), at)
	if err := os.Mkdir(filepath.Join(r.root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.root, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.svc.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		"g1": {GameID: "g1", Count: 2, SizeBytes: 2},
		"g2": {GameID: "g2", Count: 1, SizeBytes: 1},
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

func TestUsageFailures(t *testing.T) {
	t.Run("root folder missing is empty", func(t *testing.T) {
		r := newRig(t, fakeGames{})
		if err := os.Remove(r.root); err != nil {
			t.Fatal(err)
		}
		got, err := r.svc.Usage(context.Background())
		if err != nil || len(got) != 0 {
			t.Fatalf("Usage = %v, %v; want empty", got, err)
		}
	})
	t.Run("root is a file", func(t *testing.T) {
		r := newRig(t, fakeGames{})
		if err := os.Remove(r.root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(r.root, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := r.svc.Usage(context.Background()); err == nil {
			t.Fatal("a root that cannot be read must be an error, not an empty list")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		r := newRig(t, fakeGames{})
		writeFile(t, filepath.Join(r.root, "g1", "a.png"), time.Now())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := r.svc.Usage(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Usage = %v, want context.Canceled", err)
		}
	})
	t.Run("library not configured", func(t *testing.T) {
		r := newRig(t, fakeGames{})
		r.root = ""
		if _, err := r.svc.Usage(context.Background()); !errors.Is(err, ErrNoLibrary) {
			t.Fatalf("Usage = %v, want ErrNoLibrary", err)
		}
	})
}
