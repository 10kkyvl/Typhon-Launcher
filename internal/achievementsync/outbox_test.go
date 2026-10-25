package achievementsync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
)

var t0 = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func ent(igdb int64, key string, at time.Time) entry {
	return entry{IGDBID: igdb, SteamAppID: 620, Key: key, UnlockedAt: at}
}

func TestOpenOutbox(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(t *testing.T, dir string)
		wantItems int
		wantBad   bool
	}{
		{"no file starts empty", func(*testing.T, string) {}, 0, false},
		{"stored entries are loaded", func(t *testing.T, dir string) {
			if err := storage.Save(filepath.Join(dir, storeFile), storeVersion, state{Entries: []entry{ent(1, "A", t0)}}); err != nil {
				t.Fatal(err)
			}
		}, 1, false},
		{"bad json breaks the outbox", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, storeFile), []byte("{not json"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, 0, true},
		{"partial write breaks the outbox", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, storeFile), []byte(`{"version":1,"data":{"entries":[{"igdbId":1,`), 0o600); err != nil {
				t.Fatal(err)
			}
		}, 0, true},
		{"newer version breaks the outbox", func(t *testing.T, dir string) {
			if err := storage.Save(filepath.Join(dir, storeFile), storeVersion+1, state{}); err != nil {
				t.Fatal(err)
			}
		}, 0, true},
		{"unreadable path breaks the outbox", func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, storeFile), 0o700); err != nil {
				t.Fatal(err)
			}
		}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.prepare(t, dir)
			ob := openOutbox(dir, storage.Save)
			if (ob.broken != nil) != tt.wantBad || len(ob.items) != tt.wantItems {
				t.Fatalf("broken=%v items=%d", ob.broken, len(ob.items))
			}
			if !tt.wantBad {
				return
			}
			if !errors.Is(ob.broken, ErrOutboxBroken) {
				t.Fatalf("broken = %v", ob.broken)
			}
			before, errBefore := os.ReadFile(filepath.Join(dir, storeFile))
			if err := ob.add([]entry{ent(2, "B", t0)}, t0); !errors.Is(err, ErrOutboxBroken) {
				t.Fatalf("add on a broken outbox = %v", err)
			}
			after, errAfter := os.ReadFile(filepath.Join(dir, storeFile))
			if string(before) != string(after) || (errBefore == nil) != (errAfter == nil) {
				t.Fatal("the unreadable file was overwritten")
			}
		})
	}
}

func TestOpenOutboxNoFileIsNotBroken(t *testing.T) {
	ob := openOutbox(t.TempDir(), storage.Save)
	if ob.broken != nil {
		t.Fatalf("broken = %v", ob.broken)
	}
	if _, err := os.Stat(ob.path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("opening wrote a file: %v", err)
	}
}

func TestOutboxAddPersistsAndSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	ob := openOutbox(dir, storage.Save)
	if err := ob.add([]entry{ent(1, "A", t0), ent(1, "A", t0), ent(1, "B", t0)}, t0); err != nil {
		t.Fatal(err)
	}
	if len(ob.items) != 2 {
		t.Fatalf("duplicates were kept: %+v", ob.items)
	}
	again := openOutbox(dir, storage.Save)
	if again.broken != nil || len(again.items) != 2 {
		t.Fatalf("reopened = %v %+v", again.broken, again.items)
	}
}

func TestOutboxWriteFailureRollsBack(t *testing.T) {
	failing := errors.New("disk full")
	ob := &outbox{path: filepath.Join(t.TempDir(), storeFile), write: func(string, int, any) error { return failing }}
	ob.items = []entry{ent(1, "A", t0)}
	if err := ob.add([]entry{ent(1, "B", t0)}, t0); !errors.Is(err, failing) {
		t.Fatalf("add = %v", err)
	}
	if len(ob.items) != 1 || ob.items[0].Key != "A" {
		t.Fatalf("memory diverged from disk: %+v", ob.items)
	}
	if err := ob.settle([]entry{ent(1, "A", t0)}, t0); !errors.Is(err, failing) {
		t.Fatalf("settle = %v", err)
	}
	if len(ob.items) != 1 {
		t.Fatalf("settle diverged: %+v", ob.items)
	}
}

func TestOutboxWriteIntoMissingParentFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file-in-the-way")
	if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ob := openOutbox(dir, storage.Save)
	if err := ob.add([]entry{ent(1, "A", t0)}, t0); err == nil {
		t.Fatal("add succeeded though the directory cannot exist")
	}
	if len(ob.items) != 0 {
		t.Fatalf("memory kept an unsaved entry: %+v", ob.items)
	}
}

func TestOutboxDropsExpiredAndCapsOldestFirst(t *testing.T) {
	ob := openOutbox(t.TempDir(), storage.Save)
	var add []entry
	for i := range maxEntries + 20 {
		add = append(add, ent(1, fmt.Sprintf("K%03d", i), t0.Add(-time.Duration(i)*time.Minute)))
	}
	add = append(add, ent(2, "OLD", t0.Add(-maxAge-time.Second)))
	if err := ob.add(add, t0); err != nil {
		t.Fatal(err)
	}
	if len(ob.items) != maxEntries {
		t.Fatalf("len = %d", len(ob.items))
	}
	for _, it := range ob.items {
		if it.Key == "OLD" || it.Key == fmt.Sprintf("K%03d", maxEntries+19) {
			t.Fatalf("kept %s", it.Key)
		}
	}
}
