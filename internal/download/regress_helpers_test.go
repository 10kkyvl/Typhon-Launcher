package download

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

type tFile struct {
	path string
	size int
}

func buildTorrent(t *testing.T, name string, files []tFile) (*metainfo.MetaInfo, string) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, name)
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		payload := make([]byte, f.size)
		if _, err := rand.Read(payload); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	info := metainfo.Info{PieceLength: 16 << 10}
	if err := info.BuildFromFilePath(root); err != nil {
		t.Fatalf("build info: %v", err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	return &metainfo.MetaInfo{InfoBytes: infoBytes}, parent
}

func writeTorrentFile(t *testing.T, mi *metainfo.MetaInfo) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.torrent")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := mi.Write(f); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func rawTorrent(t *testing.T, info metainfo.Info) *metainfo.MetaInfo {
	t.Helper()
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	return &metainfo.MetaInfo{InfoBytes: infoBytes}
}

func reopen(t *testing.T, m *Manager) *Manager {
	t.Helper()
	m.mu.Lock()
	pc := m.pieceCompletion
	m.pieceCompletion = nil
	dir := m.store.dir
	m.mu.Unlock()
	if pc != nil {
		if err := pc.Close(); err != nil {
			t.Fatalf("close piece completion: %v", err)
		}
	}
	next := mustManagerAt(t, dir)
	next.mu.Lock()
	err := next.loadLocked()
	next.mu.Unlock()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return next
}

func persistedRecords(t *testing.T, m *Manager) []record {
	t.Helper()
	records, err := m.store.load()
	if err != nil {
		t.Fatalf("load persisted downloads: %v", err)
	}
	return records
}

func persistedStatuses(t *testing.T, m *Manager) map[string]Status {
	t.Helper()
	out := map[string]Status{}
	for _, r := range persistedRecords(t, m) {
		out[r.ID] = r.Status
	}
	return out
}

func blockDownloadsFile(t *testing.T, m *Manager) {
	t.Helper()
	path := filepath.Join(m.store.dir, "downloads.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func setItem(m *Manager, id string, edit func(d *Download)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	edit(m.findLocked(id))
}

func attachFake(m *Manager, id string) *fakeTorrent {
	eng := &fakeTorrent{size: 100}
	m.mu.Lock()
	m.engines[id] = eng
	m.mu.Unlock()
	return eng
}
