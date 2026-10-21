package achievements

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
)

func TestOpenStoreMissingFileStartsEmpty(t *testing.T) {
	st := openStore(t.TempDir(), storage.Save)
	if st.broken != nil || len(st.games) != 0 {
		t.Fatalf("broken %v games %v", st.broken, st.games)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := openStore(dir, storage.Save)
	rec := record{AppID: "620", Unlocked: map[string]time.Time{"A": at(1700000000)}, Found: true, Schema: []SchemaItem{{Key: "A", Name: "Alpha"}}}
	if err := st.commit("g1", rec); err != nil {
		t.Fatalf("commit: %v", err)
	}
	again := openStore(dir, storage.Save)
	if again.broken != nil {
		t.Fatalf("reopen: %v", again.broken)
	}
	got := again.games["g1"]
	if got.AppID != "620" || !got.Found || !got.Unlocked["A"].Equal(at(1700000000)) || got.Schema[0].Name != "Alpha" {
		t.Fatalf("got %+v", got)
	}
}

func TestStoreUnreadableFileIsNotAnEmptyState(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{"broken json", func(t *testing.T, path string) { writeFile(t, path, `{"version":1,"data":{"games":`) }},
		{"not json", func(t *testing.T, path string) { writeFile(t, path, `garbage`) }},
		{"empty file", func(t *testing.T, path string) { writeFile(t, path, ``) }},
		{"future version", func(t *testing.T, path string) { writeFile(t, path, `{"version":99,"data":{"games":{}}}`) }},
		{"not a file", func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, storeFile)
			tc.write(t, path)
			before, readErr := os.ReadFile(path)

			st := openStore(dir, storage.Save)
			if !errors.Is(st.broken, ErrStoreBroken) {
				t.Fatalf("broken = %v, want ErrStoreBroken", st.broken)
			}
			if err := st.commit("g1", record{AppID: "620"}); !errors.Is(err, ErrStoreBroken) {
				t.Fatalf("commit err = %v, a broken store must refuse to save", err)
			}
			if len(st.games) != 0 {
				t.Fatalf("memory changed: %v", st.games)
			}
			after, afterErr := os.ReadFile(path)
			if (readErr == nil) != (afterErr == nil) || !bytes.Equal(before, after) {
				t.Fatalf("the unreadable file was rewritten: %q -> %q", before, after)
			}
		})
	}
}

func TestServiceOnABrokenStoreStartsButNeverSaves(t *testing.T) {
	h := newHarness(t)
	path := filepath.Join(h.configDir, storeFile)
	const broken = `{"version":1,"data":{"games":{"g1":`
	writeFile(t, path, broken)
	svc, err := New(Config{
		ConfigDir: h.configDir,
		Games:     fakeLookup{games: map[string]GameRef{"g1": {InstallDir: h.install, AppID: "620"}}},
		Schema:    h.schema,
		Language:  func() string { return "ru" },
	})
	if err != nil {
		t.Fatalf("a broken store must not stop the launcher: %v", err)
	}
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
	ctx := context.Background()
	if _, err := svc.Scan(ctx, "g1"); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("scan err = %v", err)
	}
	if _, err := svc.GameAchievements(ctx, "g1"); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("view err = %v", err)
	}
	if _, err := svc.Rescan(ctx, "g1"); !errors.Is(err, ErrStoreBroken) {
		t.Fatalf("rescan err = %v", err)
	}
	if h.schema.count() != 0 {
		t.Fatal("a read-only service must not fetch what it cannot cache")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != broken {
		t.Fatalf("the broken file was replaced: %q %v", data, err)
	}
}

func TestStoreCommitRollsBackOnWriteFailure(t *testing.T) {
	failing := errors.New("disk full")
	t.Run("new record", func(t *testing.T) {
		st := openStore(t.TempDir(), func(string, int, any) error { return failing })
		if err := st.commit("g1", record{AppID: "620"}); !errors.Is(err, failing) {
			t.Fatalf("err = %v", err)
		}
		if _, ok := st.games["g1"]; ok {
			t.Fatal("a record that was never saved stayed in memory")
		}
	})
	t.Run("existing record", func(t *testing.T) {
		st := openStore(t.TempDir(), storage.Save)
		if err := st.commit("g1", record{AppID: "620", Found: true}); err != nil {
			t.Fatal(err)
		}
		st.write = func(string, int, any) error { return failing }
		if err := st.commit("g1", record{AppID: "730"}); !errors.Is(err, failing) {
			t.Fatalf("err = %v", err)
		}
		if got := st.games["g1"]; got.AppID != "620" || !got.Found {
			t.Fatalf("memory holds %+v, the disk holds the first record", got)
		}
	})
}
