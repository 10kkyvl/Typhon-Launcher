package download

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

type lostCache struct {
	name   string
	damage func(t *testing.T, s *store, hash string)
}

var lostCaches = []lostCache{
	{"no cached torrent", func(*testing.T, *store, string) {}},
	{"cached torrent is garbage", func(t *testing.T, s *store, hash string) {
		writeCacheBytes(t, s, hash, []byte("this is not bencode"))
	}},
	{"cached torrent has an info that cannot be read", func(t *testing.T, s *store, hash string) {
		var buf bytes.Buffer
		if err := (&metainfo.MetaInfo{InfoBytes: []byte("le")}).Write(&buf); err != nil {
			t.Fatal(err)
		}
		writeCacheBytes(t, s, hash, buf.Bytes())
	}},
}

func writeCacheBytes(t *testing.T, s *store, hash string, body []byte) {
	t.Helper()
	path := s.metainfoPath(hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func storedRecord(hash string, selected []int, total int64) record {
	return record{
		ID:          "a",
		Name:        "Game",
		Type:        TypeTorrent,
		Source:      "magnet:?xt=urn:btih:" + hash,
		InfoHash:    hash,
		Destination: filepath.Join(os.TempDir(), "typhon-selection-test"),
		Status:      StatusPaused,
		Selected:    selected,
		Total:       total,
		AddedAt:     time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
}

func loadStored(t *testing.T, dir string) *Manager {
	t.Helper()
	m := mustManagerAt(t, dir)
	m.mu.Lock()
	err := m.loadLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return m
}

func persistNow(t *testing.T, m *Manager) {
	t.Helper()
	m.mu.Lock()
	err := m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
}

func TestSelectionOutlivesAnUnreadableTorrentCache(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lostCaches {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			s := newStore(dir)
			if err := s.save([]record{storedRecord(hash, []int{1}, 30000)}); err != nil {
				t.Fatal(err)
			}
			c.damage(t, s, hash)

			m := loadStored(t, dir)
			persistNow(t, m)

			if got := persistedRecords(t, m)[0].Selected; !slices.Equal(got, []int{1}) {
				t.Fatalf("selection after a save with no torrent to read = %v, want [1]: the next restart would download everything", got)
			}
			if d := mustGet(t, m, "a"); d.Total != 30000 {
				t.Fatalf("total = %d, want the recorded 30000 until the file list is known", d.Total)
			}

			second := loadStored(t, dir)
			persistNow(t, second)
			if got := persistedRecords(t, second)[0].Selected; !slices.Equal(got, []int{1}) {
				t.Fatalf("selection after a second round trip = %v, want [1]", got)
			}

			second.settleRestored(t.Context(), restoreJob{id: "a", paused: true, trusted: true, gen: second.gen}, &fakeTorrent{size: 100}, &info)

			d := mustGet(t, second, "a")
			if len(d.Files) != 3 {
				t.Fatalf("files = %+v", d.Files)
			}
			for i, want := range []bool{false, true, false} {
				if d.Files[i].Selected != want {
					t.Errorf("file %d (%s) selected = %v, want %v: the saved selection must be applied, never widened", i, d.Files[i].Path, d.Files[i].Selected, want)
				}
			}
			if d.Total != 30000 {
				t.Errorf("total = %d, want 30000: only the selected file counts", d.Total)
			}
			if d.Status != StatusPaused {
				t.Errorf("status = %s", d.Status)
			}
			if got := persistedRecords(t, second)[0].Selected; !slices.Equal(got, []int{1}) {
				t.Errorf("persisted selection after the restore = %v, want [1]", got)
			}
		})
	}
}

func TestARecordWithoutASelectionStaysWithoutOne(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := newStore(dir).save([]record{storedRecord(hash, nil, 60000)}); err != nil {
		t.Fatal(err)
	}

	m := loadStored(t, dir)
	persistNow(t, m)

	raw, err := os.ReadFile(filepath.Join(dir, "downloads.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"selected": null`) {
		t.Fatalf("a record that never named its files was rewritten as one that selects none:\n%s", raw)
	}

	m.settleRestored(t.Context(), restoreJob{id: "a", paused: true, trusted: true, gen: m.gen}, &fakeTorrent{size: 100}, &info)

	d := mustGet(t, m, "a")
	if len(d.Files) != 3 || d.Total != 60000 {
		t.Fatalf("download = %+v", d)
	}
}

func TestASelectionThatCannotBeAppliedFailsTheDownloadInsteadOfGuessing(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		selected []int
	}{
		{"an empty selection", []int{}},
		{"an index outside the file list", []int{1, 7}},
		{"a negative index", []int{-1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := newStore(dir).save([]record{storedRecord(hash, c.selected, 30000)}); err != nil {
				t.Fatal(err)
			}
			m := loadStored(t, dir)
			eng := &fakeTorrent{size: 100}

			m.settleRestored(t.Context(), restoreJob{id: "a", paused: true, trusted: true, gen: m.gen}, eng, &info)

			d := mustGet(t, m, "a")
			if d.Status != StatusFailed || !strings.Contains(d.Error, errSelectionLost.Error()) {
				t.Fatalf("download = %s %q, want failed with the lost selection named", d.Status, d.Error)
			}
			if len(d.Files) != 0 {
				t.Fatalf("files = %+v: a selection that cannot be applied must not turn into every file", d.Files)
			}
			if !eng.wasDropped() || hasEngine(m, "a") {
				t.Fatal("the engine of a download that cannot start was kept")
			}
			if got := persistedRecords(t, m)[0]; got.Status != StatusFailed {
				t.Fatalf("persisted status = %s", got.Status)
			}
		})
	}

	t.Run("a finished download keeps its status when only its list of files is unusable", func(t *testing.T) {
		dir := t.TempDir()
		stored := storedRecord(hash, []int{}, 60000)
		stored.Status = StatusCompleted
		stored.Seeding = true
		if err := newStore(dir).save([]record{stored}); err != nil {
			t.Fatal(err)
		}
		m := loadStored(t, dir)

		m.settleRestored(t.Context(), restoreJob{id: "a", complete: true, seeding: true, gen: m.gen}, &fakeTorrent{size: 100}, &info)

		d := mustGet(t, m, "a")
		if d.Status != StatusCompleted || d.Total != 60000 {
			t.Fatalf("download = %s total=%d: the data is complete whatever became of the selection", d.Status, d.Total)
		}
	})
}

func TestAFileTorrentIsNotAddedWhenItsMetadataCannotBeKept(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	source := writeTorrentFile(t, mi)
	m := managerWithClient(t, 1)
	if _, err := m.FetchMetadata(source); err != nil {
		t.Fatal(err)
	}
	blockCacheDir(t, m)

	_, err := m.StartDownloadFrom(hash, t.TempDir(), []int{1}, Origin{})

	if !errors.Is(err, errMetainfoNotSaved) {
		t.Fatalf("error = %v, want errMetainfoNotSaved: this download could never be restored", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("downloads tracked = %d", got)
	}
	if got := persistedRecords(t, m); len(got) != 0 {
		t.Fatalf("persisted %+v", got)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents in the client = %d: a refused download must not keep downloading", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d, want both released so the hash is not stuck busy", pending, reserved)
	}
}

func TestAddTaskRefusesAFileTorrentItCannotCache(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	m := managerWithClient(t, 1)
	blockCacheDir(t, m)

	_, err := m.AddTask(t.Context(), AddRequest{Source: writeTorrentFile(t, mi), Destination: t.TempDir()})

	if !errors.Is(err, errMetainfoNotSaved) {
		t.Fatalf("error = %v, want errMetainfoNotSaved", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("downloads tracked = %d", got)
	}
	if got := persistedRecords(t, m); len(got) != 0 {
		t.Fatalf("persisted %+v", got)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents in the client = %d", got)
	}
	if _, reserved := m.bookkeeping(); reserved != 0 {
		t.Fatalf("reserved = %d: the hash is stuck busy", reserved)
	}
}

func blockCacheDir(t *testing.T, m *Manager) {
	t.Helper()
	dir := filepath.Join(m.store.dir, "torrents")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("a file where the cache directory belongs"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAMagnetDownloadIsAddedWhenOnlyTheMetadataCacheFailsAndKeepsItsSelection(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 1)
	lt, err := m.client.addMetainfo(mi, m.client.metaDir, storageOpts{})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.pending[hash] = &pending{torrent: lt, source: "magnet:?xt=urn:btih:" + hash}
	m.mu.Unlock()
	blockCacheDir(t, m)

	d, err := m.StartDownloadFrom(hash, t.TempDir(), []int{1}, Origin{})
	if err != nil {
		t.Fatalf("a magnet link can be resolved again, so a missing cache is no reason to refuse: %v", err)
	}
	m.wg.Wait()

	if got := persistedRecords(t, m)[0]; got.ID != d.ID || !slices.Equal(got.Selected, []int{1}) {
		t.Fatalf("persisted record = %+v", got)
	}
	if m.store.hasMetainfo(hash) {
		t.Fatal("setup: the cache was written although its directory is blocked")
	}

	again := loadStored(t, m.store.dir)
	if got := mustGet(t, again, d.ID); len(got.Files) != 0 || got.Total != 30000 {
		t.Fatalf("download after a restart with no cache = %+v", got)
	}
	persistNow(t, again)
	if got := persistedRecords(t, again)[0].Selected; !slices.Equal(got, []int{1}) {
		t.Fatalf("selection after a restart with no cache = %v, want [1]", got)
	}
}

func TestAStoredSelectionTheCachedTorrentCannotApplyIsHeldNotWidened(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	dir := t.TempDir()
	s := newStore(dir)
	if err := s.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	if err := s.save([]record{storedRecord(hash, []int{}, 30000)}); err != nil {
		t.Fatal(err)
	}

	m := loadStored(t, dir)

	d := mustGet(t, m, "a")
	if len(d.Files) != 0 || d.Total != 30000 {
		t.Fatalf("download = %+v: an empty selection must not become either every file or none", d)
	}
}
