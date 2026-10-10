package download

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestStoreWithoutADirectoryRefusesEverything(t *testing.T) {
	s := newStore("")
	mi, _ := threeFileTorrent(t)

	if _, err := s.load(); err == nil {
		t.Error("load of a store without a directory returned a queue")
	}
	if err := s.save([]record{{ID: "a"}}); err == nil {
		t.Error("save of a store without a directory succeeded: it would write relative to the working directory")
	}
	if err := s.saveMetainfo("aaaa", mi); err == nil {
		t.Error("saveMetainfo of a store without a directory succeeded")
	}
	if _, err := s.loadMetainfo("aaaa"); err == nil {
		t.Error("loadMetainfo of a store without a directory succeeded")
	}
	if s.hasMetainfo("aaaa") {
		t.Error("hasMetainfo of a store without a directory is true")
	}
	s.sweepMetainfo(map[string]bool{})
	s.removeMetainfo("aaaa")
}

func TestUnreadableDownloadsFileIsNotAnEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "downloads.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	s := newStore(dir)

	records, err := s.load()

	if err == nil {
		t.Fatalf("load = %+v, nil: an unreadable file is not the same as no file", records)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v: only a missing file may mean an empty queue", err)
	}
	m := mustManagerAt(t, dir)
	if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("the manager started over a downloads file it cannot read")
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		t.Fatalf("the unreadable path was replaced: %v", err)
	}
}

func TestMetainfoCacheRoundTripAndSweep(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	s := newStore(t.TempDir())

	if _, err := s.loadMetainfo(hash); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a cache miss = %v, want fs.ErrNotExist: callers tell a miss from a broken file by it", err)
	}
	if err := s.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	got, err := s.loadMetainfo(hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.HashInfoBytes() != mi.HashInfoBytes() {
		t.Fatal("the cached torrent has another infohash")
	}
	if !s.hasMetainfo(hash) {
		t.Fatal("hasMetainfo is false for a saved torrent")
	}

	dir := filepath.Join(s.dir, "torrents")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "stray.torrent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ORPHAN.torrent"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.sweepMetainfo(map[string]bool{hash: true})

	for name, want := range map[string]bool{hash + ".torrent": true, "notes.txt": true, "stray.torrent": true, "ORPHAN.torrent": false} {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s present = %v, want %v", name, err == nil, want)
		}
	}

	s.removeMetainfo(hash)
	s.removeMetainfo(hash)
	if s.hasMetainfo(hash) {
		t.Fatal("removed torrent still cached")
	}
}
