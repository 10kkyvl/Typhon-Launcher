package download

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errBadOrigin = errors.New("bad origin")

func refuseImages(origin Origin) error {
	if origin.Platform != "" {
		return errBadOrigin
	}
	return nil
}

func TestRefusedOriginStopsTheStartAndKeepsTheMetadata(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 1)
	m.SetOriginCheck(refuseImages)
	if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
		t.Fatal(err)
	}

	if _, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{Platform: "ps9"}); !errors.Is(err, errBadOrigin) {
		t.Fatalf("start = %v", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("a refused start left %d downloads behind", got)
	}
	if got := persistedRecords(t, m); len(got) != 0 {
		t.Fatalf("a refused start persisted %+v", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 1 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d", pending, reserved)
	}

	d, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})
	if err != nil {
		t.Fatalf("retry with a valid origin: %v", err)
	}
	if d.Origin.Platform != "" {
		t.Fatalf("origin = %+v", d.Origin)
	}
	m.wg.Wait()
}

func TestRefusedOriginStopsAddTask(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	m := managerWithClient(t, 1)
	m.SetOriginCheck(refuseImages)
	dest := filepath.Join(t.TempDir(), "not created")

	_, err := m.AddTask(t.Context(), AddRequest{
		Source:      writeTorrentFile(t, mi),
		Destination: dest,
		Origin:      Origin{Platform: "ps9"},
	})

	if !errors.Is(err, errBadOrigin) {
		t.Fatalf("add task = %v", err)
	}
	if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination was touched: %v", statErr)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("downloads = %d", got)
	}
}

func TestImageOriginAndAttemptSurviveARestart(t *testing.T) {
	first := newTestManager(t, 1)
	first.addTestItem("a", StatusCompleted)
	first.addTestItem("b", StatusCompleted)
	origin := Origin{ReleaseID: "r1", Platform: "ps2", Title: "Console Game", Cover: "https://example.com/c.jpg"}
	first.mu.Lock()
	first.findLocked("a").Origin = origin
	first.findLocked("b").Origin = origin
	if err := first.persistLocked(); err != nil {
		first.mu.Unlock()
		t.Fatal(err)
	}
	first.mu.Unlock()
	if err := first.MarkImageAttempted("b"); err != nil {
		t.Fatal(err)
	}

	second := reopen(t, first)

	for id, attempted := range map[string]bool{"a": false, "b": true} {
		got := mustGet(t, second, id)
		if got.Origin.Platform != origin.Platform || got.Origin.Title != origin.Title || got.Origin.Cover != origin.Cover {
			t.Errorf("%s: origin = %+v", id, got.Origin)
		}
		if got.ImageAttempted != attempted {
			t.Errorf("%s: attempted = %v", id, got.ImageAttempted)
		}
	}
}

func TestDownloadsFileWithoutImageFieldsLoads(t *testing.T) {
	legacy := `[{"id": "old1", "name": "Old", "source": "magnet:?xt=urn:btih:aaaa", "infoHash": "aaaa",
	  "destination": "D:\\Games", "status": "completed", "selected": null, "downloaded": 5, "total": 5,
	  "seeding": false, "origin": {"releaseId": "r1", "gameId": "g1"}, "addedAt": "2026-01-02T03:04:05Z",
	  "completedAt": "2026-01-02T04:00:00Z", "error": ""}]`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "downloads.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	m := mustManagerAt(t, dir)
	m.mu.Lock()
	err := m.loadLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	got := mustGet(t, m, "old1")

	if got.Origin.ReleaseID != "r1" || got.Origin.Platform != "" || got.Origin.Title != "" || got.Origin.Cover != "" || got.ImageAttempted {
		t.Fatalf("download = %+v", got)
	}
}

func TestMarkImageAttempted(t *testing.T) {
	t.Run("unknown download", func(t *testing.T) {
		m := newTestManager(t, 1)
		if err := m.MarkImageAttempted("gone"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("mark = %v", err)
		}
	})
	t.Run("a failed save leaves memory as it was", func(t *testing.T) {
		m := newTestManager(t, 1)
		m.addTestItem("a", StatusCompleted)
		failure := errors.New("unread")
		m.mu.Lock()
		m.loadErr = failure
		m.mu.Unlock()
		if err := m.MarkImageAttempted("a"); !errors.Is(err, failure) {
			t.Fatalf("mark = %v", err)
		}
		if mustGet(t, m, "a").ImageAttempted {
			t.Fatal("memory says tried, the disk does not")
		}
	})
}
