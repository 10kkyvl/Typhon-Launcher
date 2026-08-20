package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	s := newStore(t.TempDir())
	completedAt := time.Now().Truncate(time.Second)
	records := []record{
		{
			ID:          "a",
			Name:        "Game A",
			Type:        TypeTorrent,
			Source:      "magnet:?xt=urn:btih:aaaa",
			InfoHash:    "aaaa",
			Destination: `D:\Games`,
			Status:      StatusDownloading,
			Selected:    []int{0, 2},
			Downloaded:  512,
			Total:       2048,
			AddedAt:     time.Now().Truncate(time.Second),
		},
		{
			ID:          "b",
			Name:        "Game B",
			Type:        TypeTorrent,
			InfoHash:    "bbbb",
			Status:      StatusCompleted,
			Seeding:     true,
			CompletedAt: &completedAt,
		},
	}
	if err := s.save(records); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.listPath()); err != nil {
		t.Fatalf("downloads.json missing: %v", err)
	}

	loaded := newStore(s.dir).load()
	if len(loaded) != 2 {
		t.Fatalf("loaded %d records, want 2", len(loaded))
	}
	if loaded[0].ID != "a" || loaded[1].ID != "b" {
		t.Fatalf("order not preserved: %v", loaded)
	}
	if len(loaded[0].Selected) != 2 || loaded[0].Selected[1] != 2 {
		t.Fatalf("selection = %v", loaded[0].Selected)
	}
	if loaded[1].CompletedAt == nil || !loaded[1].CompletedAt.Equal(completedAt) {
		t.Fatalf("completedAt = %v, want %v", loaded[1].CompletedAt, completedAt)
	}
}

func TestStoreSaveLeavesNoTempFile(t *testing.T) {
	s := newStore(t.TempDir())
	if err := s.save([]record{{ID: "a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.listPath() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestStoreCorruptFileLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "downloads.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := newStore(dir).load(); len(got) != 0 {
		t.Fatalf("loaded %d records, want 0", len(got))
	}
}

func TestStoreMissingFileLoadsEmpty(t *testing.T) {
	if got := newStore(t.TempDir()).load(); got != nil {
		t.Fatalf("loaded %v, want nil", got)
	}
}
