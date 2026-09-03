package playlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/storage"
)

func TestRecordPersistsAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	old := now.Add(-Retention - time.Hour)
	s.Record("old", old.Add(-time.Hour), old)
	s.Record("fresh", now.Add(-2*time.Hour), now.Add(-time.Hour))
	s.Record("", now.Add(-time.Hour), now)
	s.Record("backwards", now, now.Add(-time.Minute))
	s.Record("zero", now, now)

	got := s.Since(time.Time{})
	if len(got) != 1 || got[0].GameID != "fresh" {
		t.Fatalf("sessions = %+v, want only fresh", got)
	}

	var onDisk []Session
	if err := storage.Load(path, playlogVersion, nil, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk) != 1 || onDisk[0].GameID != "fresh" {
		t.Fatalf("on disk = %+v, want only fresh", onDisk)
	}

	reloaded, err := NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Since(time.Time{})) != 1 {
		t.Fatalf("reload lost sessions")
	}
}

func TestSinceFiltersAndSorts(t *testing.T) {
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "playlog.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	s.Record("b", now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	s.Record("a", now.Add(-6*time.Hour), now.Add(-5*time.Hour))
	s.Record("c", now.Add(-time.Hour), now)

	got := s.Since(now.Add(-4 * time.Hour))
	if len(got) != 2 || got[0].GameID != "b" || got[1].GameID != "c" {
		t.Fatalf("sessions = %+v, want [b c] sorted by start", got)
	}
	got[0].GameID = "mutated"
	if s.Since(time.Time{})[1].GameID == "mutated" {
		t.Fatalf("Since must return a copy")
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "none", "playlog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Since(time.Time{})) != 0 {
		t.Fatal("expected empty log")
	}
}

func TestCorruptFileFailsConstructor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlog.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); err == nil {
		t.Fatal("corrupt log must fail loudly")
	}
}
