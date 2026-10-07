package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/usagestats"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type usageLog struct {
	mu     sync.Mutex
	events []usagestats.Event
}

func (u *usageLog) record(ev usagestats.Event) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.events = append(u.events, ev)
}

func (u *usageLog) types() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0, len(u.events))
	for _, e := range u.events {
		out = append(out, e.Type)
	}
	return out
}

func threeFileTorrent(t *testing.T) (*metainfo.MetaInfo, string) {
	t.Helper()
	return buildTorrent(t, "Game", []tFile{{"a.bin", 20000}, {"b.bin", 30000}, {"c.bin", 10000}})
}

func managerWithClient(t *testing.T, max int) *Manager {
	t.Helper()
	m := newTestManager(t, max)
	m.client = offlineClient(t)
	return m
}

func (m *Manager) bookkeeping() (pending, reserved int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending), len(m.reserved)
}

func TestStartDownloadFromTurnsFetchedMetadataIntoARunningDownload(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	source := writeTorrentFile(t, mi)

	m := managerWithClient(t, 2)
	log := recordEmits(t)
	usage := &usageLog{}
	m.SetUsageRecorder(usage.record)
	started := make(chan Download, 1)
	m.SetOnStarted(func(d Download) { started <- d })

	info, err := m.FetchMetadata(source)
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if info.InfoHash != hash || info.Name != "Game" || info.TotalBytes != 60000 || len(info.Files) != 3 {
		t.Fatalf("metadata = %+v", info)
	}
	for _, f := range info.Files {
		if !f.Selected {
			t.Fatalf("file %s is not selected by default", f.Path)
		}
	}
	if !m.store.hasMetainfo(hash) {
		t.Fatal("fetched metadata was not cached")
	}

	dest := filepath.Join(t.TempDir(), "games", "new")
	d, err := m.StartDownloadFrom(hash, "  "+dest+"  ", []int{1}, Origin{GameID: "game-1", Version: "2.0"})
	if err != nil {
		t.Fatalf("StartDownloadFrom: %v", err)
	}

	if d.Status != StatusDownloading || d.Name != "Game" || d.Type != TypeTorrent || d.Source != source || d.InfoHash != hash {
		t.Fatalf("download = %+v", d)
	}
	if d.Destination != dest {
		t.Fatalf("destination = %q, want it trimmed to %q", d.Destination, dest)
	}
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		t.Fatalf("destination was not created: %v", err)
	}
	if d.Total != 30000 || d.Origin.GameID != "game-1" || d.Origin.Version != "2.0" || d.ETASeconds != -1 || d.AddedAt.IsZero() {
		t.Fatalf("download numbers = %+v", d)
	}
	for i, wantSelected := range []bool{false, true, false} {
		if d.Files[i].Selected != wantSelected {
			t.Errorf("file %d selected = %v, want %v", i, d.Files[i].Selected, wantSelected)
		}
	}

	m.mu.Lock()
	eng := m.engines[d.ID]
	m.mu.Unlock()
	lt, ok := eng.(*liveTorrent)
	if !ok {
		t.Fatalf("engine = %T", eng)
	}
	for i, wantPriority := range []torrent.PiecePriority{torrent.PiecePriorityNone, torrent.PiecePriorityNormal, torrent.PiecePriorityNone} {
		if got := lt.t.Files()[i].Priority(); got != wantPriority {
			t.Errorf("engine priority of file %d = %v, want %v", i, got, wantPriority)
		}
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents in the client = %d, want exactly the download (the pending probe must be gone)", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d, want both released", pending, reserved)
	}

	records := persistedRecords(t, m)
	if len(records) != 1 {
		t.Fatalf("persisted %d records", len(records))
	}
	rec := records[0]
	if rec.ID != d.ID || rec.Source != source || rec.Destination != dest || rec.Total != 30000 ||
		len(rec.Selected) != 1 || rec.Selected[0] != 1 || rec.Origin.GameID != "game-1" {
		t.Fatalf("persisted record = %+v", rec)
	}

	select {
	case got := <-started:
		if got.ID != d.ID {
			t.Fatalf("started callback got %s", got.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the started callback never ran")
	}
	if got := usage.types(); len(got) != 1 || got[0] != usagestats.TypeDownloadStarted {
		t.Fatalf("usage events = %v", got)
	}
	if log.count(eventAdded) != 1 {
		t.Fatalf("download:added emitted %d times", log.count(eventAdded))
	}

	if _, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{}); !errors.Is(err, errMetadataRequired) {
		t.Fatalf("second start of the same metadata = %v, want errMetadataRequired", err)
	}
	if _, err := m.FetchMetadata(source); !errors.Is(err, errDuplicateDownload) {
		t.Fatalf("fetching a torrent that is already downloading = %v, want errDuplicateDownload", err)
	}
	m.wg.Wait()
}

func TestStartDownloadFromRefusalsKeepTheMetadataForARetry(t *testing.T) {
	cases := []struct {
		name string
		dest func(t *testing.T) string
		sel  []int
		want error
	}{
		{"blank destination", func(*testing.T) string { return "   " }, nil, errEmptyDestination},
		{"destination below a file", func(t *testing.T) string {
			file := filepath.Join(t.TempDir(), "occupied")
			if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(file, "sub")
		}, nil, errDestUnavailable},
		{"empty selection", func(t *testing.T) string { return t.TempDir() }, []int{}, errNoFilesSelected},
		{"selection outside the file list", func(t *testing.T) string { return t.TempDir() }, []int{7, -1}, errNoFilesSelected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mi, _ := threeFileTorrent(t)
			hash := mi.HashInfoBytes().HexString()
			m := managerWithClient(t, 1)
			if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
				t.Fatal(err)
			}

			_, err := m.StartDownloadFrom(hash, c.dest(t), c.sel, Origin{})
			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if got := len(m.List()); got != 0 {
				t.Fatalf("a refused start left %d downloads behind", got)
			}
			if got := persistedRecords(t, m); len(got) != 0 {
				t.Fatalf("a refused start persisted %+v", got)
			}
			if pending, reserved := m.bookkeeping(); pending != 1 || reserved != 0 {
				t.Fatalf("pending = %d, reserved = %d: the fetched metadata must wait for a corrected request", pending, reserved)
			}

			d, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})
			if err != nil {
				t.Fatalf("retry with valid arguments: %v", err)
			}
			if d.Total != 60000 {
				t.Fatalf("a nil selection must take every file, total = %d", d.Total)
			}
			m.wg.Wait()
		})
	}

	t.Run("metadata that was never fetched", func(t *testing.T) {
		m := managerWithClient(t, 1)
		_, err := m.StartDownloadFrom("a748597437835a2fd0d2e06f8edd86fee316a84d", t.TempDir(), nil, Origin{})
		if !errors.Is(err, errMetadataRequired) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestStartDownloadFromRollsBackOnPersistFailure(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	source := writeTorrentFile(t, mi)
	m := managerWithClient(t, 1)
	if _, err := m.FetchMetadata(source); err != nil {
		t.Fatal(err)
	}
	breakDownloadsFile(t, m)

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	if err == nil {
		t.Fatal("the persist failure was swallowed")
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("downloads tracked = %d, want 0", got)
	}
	m.mu.Lock()
	engines := len(m.engines)
	m.mu.Unlock()
	if engines != 0 {
		t.Fatalf("engines left = %d", engines)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the client = %d: the rolled back download must not keep downloading", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d, want both empty so the hash is not stuck busy", pending, reserved)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded = %+v, want the failed write surfaced", st)
	}

	if err := os.Remove(filepath.Join(m.store.dir, "downloads.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FetchMetadata(source); err != nil {
		t.Fatalf("fetching again after the disk recovered: %v", err)
	}
	if _, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{}); err != nil {
		t.Fatalf("starting again after the disk recovered: %v", err)
	}
	if st := m.degradedStatus(); st.Degraded {
		t.Fatalf("degraded = %+v after a successful write", st)
	}
	m.wg.Wait()
}

func TestFetchMetadataOutcomes(t *testing.T) {
	unsafe := rawTorrent(t, metainfo.Info{
		Name:        "Game",
		PieceLength: 16 << 10,
		Pieces:      make([]byte, 20),
		Files:       []metainfo.FileInfo{{Length: 1, Path: []string{"..", "evil.bin"}}},
	})
	garbage := filepath.Join(t.TempDir(), "broken.torrent")
	if err := os.WriteFile(garbage, []byte("not a torrent"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		source func(t *testing.T) string
		want   error
	}{
		{"blank source", func(*testing.T) string { return "   " }, errEmptySource},
		{"malformed magnet", func(*testing.T) string { return "magnet:?xt=urn:btih:zzzz" }, errInvalidMagnet},
		{"missing torrent file", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope.torrent") }, errTorrentReadFailed},
		{"file that is not a torrent", func(*testing.T) string { return garbage }, errTorrentReadFailed},
		{"torrent whose file path escapes the destination", func(t *testing.T) string { return writeTorrentFile(t, unsafe) }, errBadPaths},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := managerWithClient(t, 1)

			_, err := m.FetchMetadata(c.source(t))

			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
				t.Fatalf("pending = %d, reserved = %d after a refusal", pending, reserved)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("a refused torrent stays in the client: %d", got)
			}
			if entries, err := os.ReadDir(filepath.Join(m.store.dir, "torrents")); err == nil && len(entries) != 0 {
				t.Fatalf("a refused torrent was cached: %v", entries)
			}
		})
	}

	t.Run("a torrent already downloading", func(t *testing.T) {
		mi, _ := threeFileTorrent(t)
		m := managerWithClient(t, 1)
		m.addTestItem("a", StatusPaused)
		setItem(m, "a", func(d *Download) { d.InfoHash = strings.ToUpper(mi.HashInfoBytes().HexString()) })
		if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); !errors.Is(err, errDuplicateDownload) {
			t.Fatalf("error = %v, want errDuplicateDownload whatever the case of the stored hash", err)
		}
	})

	t.Run("the same source twice adds one torrent", func(t *testing.T) {
		mi, _ := threeFileTorrent(t)
		source := writeTorrentFile(t, mi)
		m := managerWithClient(t, 1)
		first, err := m.FetchMetadata(source)
		if err != nil {
			t.Fatal(err)
		}
		second, err := m.FetchMetadata(source)
		if err != nil {
			t.Fatalf("second fetch: %v", err)
		}
		if first.InfoHash != second.InfoHash || first.Name != second.Name || len(first.Files) != len(second.Files) {
			t.Fatalf("answers differ: %+v vs %+v", first, second)
		}
		if got := len(m.client.cl.Torrents()); got != 1 {
			t.Fatalf("torrents in the client = %d", got)
		}
	})

	t.Run("discarding the metadata frees the hash and the cache", func(t *testing.T) {
		mi, _ := threeFileTorrent(t)
		hash := mi.HashInfoBytes().HexString()
		source := writeTorrentFile(t, mi)
		m := managerWithClient(t, 1)
		if _, err := m.FetchMetadata(source); err != nil {
			t.Fatal(err)
		}

		m.DiscardMetadata(hash)

		if got := len(m.client.cl.Torrents()); got != 0 {
			t.Fatalf("torrents in the client = %d", got)
		}
		if m.store.hasMetainfo(hash) {
			t.Fatal("the cache of a discarded fetch was kept")
		}
		if _, err := m.FetchMetadata(source); err != nil {
			t.Fatalf("fetching after a discard: %v", err)
		}
	})

	t.Run("cancelling a source with nothing in flight is harmless", func(t *testing.T) {
		m := managerWithClient(t, 1)
		m.CancelFetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d")
		m.CancelFetchMetadata("")
	})
}

func TestAddTaskRefusals(t *testing.T) {
	unsafe := rawTorrent(t, metainfo.Info{
		Name:        "Game",
		PieceLength: 16 << 10,
		Pieces:      make([]byte, 20),
		Files:       []metainfo.FileInfo{{Length: 1, Path: []string{"..", "evil.bin"}}},
	})
	cached := func(t *testing.T, m *Manager, mi *metainfo.MetaInfo) string {
		t.Helper()
		hash := mi.HashInfoBytes().HexString()
		if err := m.store.saveMetainfo(hash, mi); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	cases := []struct {
		name  string
		setup func(t *testing.T, m *Manager) AddRequest
		want  error
	}{
		{"no client", func(t *testing.T, m *Manager) AddRequest {
			m.client = nil
			return AddRequest{Source: "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d", Destination: t.TempDir()}
		}, errNoClient},
		{"manager shutting down", func(t *testing.T, m *Manager) AddRequest {
			mi, _ := threeFileTorrent(t)
			m.mu.Lock()
			m.closing = true
			m.mu.Unlock()
			return AddRequest{InfoHash: cached(t, m, mi), Destination: t.TempDir()}
		}, errNoClient},
		{"no source and nothing cached", func(t *testing.T, m *Manager) AddRequest {
			return AddRequest{InfoHash: "a748597437835a2fd0d2e06f8edd86fee316a84d", Destination: t.TempDir()}
		}, errNoMetadata},
		{"unsafe file paths", func(t *testing.T, m *Manager) AddRequest {
			return AddRequest{InfoHash: cached(t, m, unsafe), Destination: t.TempDir()}
		}, errBadPaths},
		{"an infohash that is already a download", func(t *testing.T, m *Manager) AddRequest {
			mi, _ := threeFileTorrent(t)
			hash := cached(t, m, mi)
			m.addTestItem("a", StatusPaused)
			setItem(m, "a", func(d *Download) { d.InfoHash = hash })
			attachFake(m, "a")
			return AddRequest{InfoHash: hash, Destination: t.TempDir()}
		}, errDuplicateTask},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := managerWithClient(t, 1)
			cl := m.client
			req := c.setup(t, m)
			before := len(m.List())

			_, err := m.AddTask(context.Background(), req)

			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if got := len(m.List()); got != before {
				t.Fatalf("downloads = %d, want %d", got, before)
			}
			if got := len(cl.cl.Torrents()); got != 0 {
				t.Fatalf("a refused task left %d torrents in the client", got)
			}
			if _, reserved := m.bookkeeping(); reserved != 0 {
				t.Fatalf("reserved = %d: the hash is stuck busy", reserved)
			}
		})
	}

	t.Run("blank destination", func(t *testing.T) {
		mi, _ := threeFileTorrent(t)
		m := managerWithClient(t, 1)
		if _, err := m.AddTask(context.Background(), AddRequest{InfoHash: cached(t, m, mi), Destination: "  "}); err == nil {
			t.Fatal("a task without a destination was accepted")
		}
		if got := len(m.List()); got != 0 {
			t.Fatalf("downloads = %d", got)
		}
	})

	t.Run("destination below a file", func(t *testing.T) {
		mi, _ := threeFileTorrent(t)
		m := managerWithClient(t, 1)
		file := filepath.Join(t.TempDir(), "occupied")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := m.AddTask(context.Background(), AddRequest{InfoHash: cached(t, m, mi), Destination: filepath.Join(file, "sub")}); err == nil {
			t.Fatal("a task below a regular file was accepted")
		}
		if got := len(m.List()); got != 0 {
			t.Fatalf("downloads = %d", got)
		}
	})
}

func TestAddTaskVerifyCompletesDataAlreadyInPlace(t *testing.T) {
	mi, parent := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 2)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	log := recordEmits(t)

	d, err := m.AddTask(context.Background(), AddRequest{
		InfoHash:    hash,
		Destination: parent,
		Name:        "Repair of Game",
		InPlace:     true,
		Verify:      true,
		Origin:      Origin{Purpose: PurposeRepair, GameID: "game-1", LibraryID: "lib-1"},
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if d.Status != StatusQueued || !d.InPlace || d.Flat || d.Name != "Repair of Game" || d.Total != 60000 {
		t.Fatalf("task = %+v", d)
	}
	m.mu.Lock()
	_, attachedEarly := m.engines[d.ID]
	m.mu.Unlock()
	if attachedEarly && m.statusOf(t, d.ID) == StatusQueued {
		t.Fatal("a task to verify joined the queue before its data was checked")
	}

	waitUntil(t, "the data to be verified and the task completed", func() bool {
		m.sample(m.ctx, time.Now())
		return m.statusOf(t, d.ID) == StatusCompleted
	})
	m.wg.Wait()

	done := mustGet(t, m, d.ID)
	if done.CompletedAt == nil || done.Progress != 1 || done.Downloaded != 60000 {
		t.Fatalf("completed task = %+v", done)
	}
	if got := log.statusesOf(d.ID); !containsStatus(got, StatusVerifying) {
		t.Fatalf("statuses seen = %v, want the data to be checked first", got)
	}
	if got := m.ByOrigin("game-1", PurposeRepair); len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("ByOrigin(game, repair) = %+v", got)
	}
	if got := m.ByOrigin("lib-1", PurposeRepair); len(got) != 1 {
		t.Fatalf("ByOrigin(library id, repair) = %+v", got)
	}
	if got := m.ByOrigin("game-1", PurposeUpdate); len(got) != 0 {
		t.Fatalf("ByOrigin(game, update) = %+v, want none", got)
	}
	if got := m.ByOrigin("other", PurposeRepair); len(got) != 0 {
		t.Fatalf("ByOrigin(other game) = %+v, want none", got)
	}
	for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
		if _, err := os.Stat(filepath.Join(parent, "Game", name)); err != nil {
			t.Fatalf("the data of an in-place task must stay where it is: %v", err)
		}
	}
}

func containsStatus(list []Status, want Status) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
