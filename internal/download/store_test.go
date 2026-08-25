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

	loaded, err := newStore(s.dir).load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
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

func TestStoreCorruptFileFailsLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "downloads.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := newStore(dir).load()
	if err == nil {
		t.Fatalf("load = %v, want error", got)
	}
}

func TestStoreMissingFileLoadsEmpty(t *testing.T) {
	got, err := newStore(t.TempDir()).load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatalf("loaded %v, want nil", got)
	}
}

func writeTestMetainfo(t *testing.T, s *store, infoHash string) {
	t.Helper()
	path := s.metainfoPath(infoHash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("d4:infod4:name1:aee"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHasMetainfo(t *testing.T) {
	s := newStore(t.TempDir())
	if s.hasMetainfo("aaaa") {
		t.Fatal("reported metainfo that was never written")
	}
	writeTestMetainfo(t, s, "aaaa")
	if !s.hasMetainfo("aaaa") {
		t.Fatal("written metainfo not found")
	}
}

func TestStoreSweepMetainfoRemovesOrphans(t *testing.T) {
	s := newStore(t.TempDir())
	writeTestMetainfo(t, s, "keep")
	writeTestMetainfo(t, s, "orphan")

	s.sweepMetainfo(map[string]bool{"keep": true})

	if !s.hasMetainfo("keep") {
		t.Fatal("known metainfo removed")
	}
	if s.hasMetainfo("orphan") {
		t.Fatal("orphaned metainfo kept")
	}
}

func TestStoreSweepMetainfoWithoutDirectory(t *testing.T) {
	newStore(t.TempDir()).sweepMetainfo(nil)
}

func TestStoreKeepsOrigin(t *testing.T) {
	s := newStore(t.TempDir())
	origin := Origin{ReleaseID: "rel-1", SourceID: "src-1", GameID: "game-1"}
	if err := s.save([]record{{ID: "a", InfoHash: "aaaa", Origin: origin}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Origin != origin {
		t.Fatalf("origin = %+v, want %+v", loaded[0].Origin, origin)
	}
}
