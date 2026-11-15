package download

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// fetchedForDiscard is fetchedTorrent with the torrent and its source, for the
// tests that fetch it again after the discard.
func fetchedForDiscard(t *testing.T) (m *Manager, mi *metainfo.MetaInfo, hash, source string) {
	t.Helper()
	mi, _ = threeFileTorrent(t)
	hash = mi.HashInfoBytes().HexString()
	source = writeTorrentFile(t, mi)
	m = managerWithClient(t, 1)
	if _, err := m.FetchMetadata(source); err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	return m, mi, hash, source
}

// discardOnFirstAdd discards the fetched torrent while Start adds it at its
// destination, the window in which Start holds the hash and no lock. then runs
// after the discard on that same add; later adds only count.
func discardOnFirstAdd(t *testing.T, m *Manager, hash string, then func(add int)) *int {
	t.Helper()
	adds := 0
	m.client.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		adds++
		if adds == 1 {
			assertHeldByStart(t, m)
			m.DiscardMetadata(hash)
		}
		if then != nil {
			then(adds)
		}
		return tiers, nil
	}
	return &adds
}

func assertHeldByStart(t *testing.T, m *Manager) {
	t.Helper()
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 1 {
		t.Errorf("at the discard pending = %d, reserved = %d: it must land while the Start holds the hash", pending, reserved)
	}
}

func assertNothingLeftOfTheDiscarded(t *testing.T, m *Manager, hash, source string, cache bool) {
	t.Helper()
	if got := len(m.List()); got != 0 {
		t.Fatalf("a failed start left %d downloads", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d: the discard asked for nothing to wait", pending, reserved)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents in the client = %d after the discard", got)
	}
	if cache && m.store.hasMetainfo(hash) {
		t.Fatal("the cached torrent of a discarded fetch was left behind")
	}
	if _, err := m.FetchMetadata(source); err != nil {
		t.Fatalf("fetching the discarded torrent again: %v", err)
	}
}

func TestADiscardDuringAFailingStartWins(t *testing.T) {
	t.Run("the destination add fails", func(t *testing.T) {
		m, mi, hash, source := fetchedForDiscard(t)
		// A torrent already in the client under the same hash makes the
		// destination add fail; it leaves before any later add so that only
		// the discard can keep the torrent from waiting.
		var intruder *torrent.Torrent
		adds := discardOnFirstAdd(t, m, hash, func(add int) {
			switch add {
			case 1:
				var err error
				if intruder, err = m.client.cl.AddTorrent(mi); err != nil {
					t.Fatalf("add the intruder: %v", err)
				}
			case 2:
				intruder.Drop()
				intruder = nil
			}
		})

		_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

		if intruder != nil {
			intruder.Drop()
		}
		if *adds < 1 {
			t.Fatal("the destination add never ran")
		}
		if got := uierr.Code(err); got != "download.add_torrent_failed" {
			t.Fatalf("code = %q (%v), want download.add_torrent_failed", got, err)
		}
		assertNothingLeftOfTheDiscarded(t, m, hash, source, true)
	})

	t.Run("the cached torrent is not written", func(t *testing.T) {
		m, _, hash, source := fetchedForDiscard(t)
		cached := m.store.metainfoPath(hash)
		if err := os.Remove(cached); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(cached, "occupied"), 0o755); err != nil {
			t.Fatal(err)
		}
		discardOnFirstAdd(t, m, hash, nil)

		_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

		if !errors.Is(err, errMetainfoNotSaved) {
			t.Fatalf("error = %v, want errMetainfoNotSaved", err)
		}
		if err := os.RemoveAll(cached); err != nil {
			t.Fatal(err)
		}
		assertNothingLeftOfTheDiscarded(t, m, hash, source, false)
	})

	t.Run("the list is not saved", func(t *testing.T) {
		m, _, hash, source := fetchedForDiscard(t)
		breakDownloadsFile(t, m)
		discardOnFirstAdd(t, m, hash, nil)

		_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

		if got := uierr.Code(err); got != "download.save_failed" {
			t.Fatalf("code = %q (%v), want download.save_failed", got, err)
		}
		assertNothingLeftOfTheDiscarded(t, m, hash, source, true)
	})

	t.Run("the discard lands while the torrent is handed back", func(t *testing.T) {
		m, _, hash, source := fetchedForDiscard(t)
		breakDownloadsFile(t, m)
		adds := 0
		m.client.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
			adds++
			if adds == 2 {
				assertHeldByStart(t, m)
				m.DiscardMetadata(hash)
			}
			return tiers, nil
		}

		_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

		if adds != 2 {
			t.Fatalf("adds = %d, want the destination add and the hand-back add", adds)
		}
		if got := uierr.Code(err); got != "download.save_failed" {
			t.Fatalf("code = %q (%v), want download.save_failed", got, err)
		}
		assertNothingLeftOfTheDiscarded(t, m, hash, source, true)
	})

	t.Run("the client changes", func(t *testing.T) {
		m, _, hash, source := fetchedForDiscard(t)
		old, next := m.client, offlineClient(t)
		discardOnFirstAdd(t, m, hash, func(add int) {
			if add == 1 {
				m.replaceClient(next)
			}
		})

		_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

		if got := uierr.Code(err); got != "download.metadata_required" || !errors.Is(err, errNetworkDown) {
			t.Fatalf("error = %v (%q), want download.metadata_required over errNetworkDown", err, got)
		}
		if got := len(old.cl.Torrents()); got != 0 {
			t.Fatalf("torrents left in the replaced client = %d", got)
		}
		assertNothingLeftOfTheDiscarded(t, m, hash, source, true)
	})
}

// onLog runs act when a record with message msg is logged: the one place a
// test can stand inside Start's pre-checks, which have no other step to wait on.
type onLog struct {
	msg string
	act func()
}

func (h *onLog) Enabled(context.Context, slog.Level) bool { return true }

func (h *onLog) Handle(_ context.Context, r slog.Record) error {
	if r.Message == h.msg {
		h.act()
	}
	return nil
}

func (h *onLog) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *onLog) WithGroup(string) slog.Handler { return h }

func TestADiscardDuringAStartPreCheckWins(t *testing.T) {
	m, _, hash, source := fetchedForDiscard(t)
	// A file where the destination folder goes makes the folder check fail.
	destination := filepath.Join(t.TempDir(), "taken")
	if err := os.WriteFile(destination, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	discarded := false
	previous := slog.Default()
	slog.SetDefault(slog.New(&onLog{msg: "create destination", act: func() {
		assertHeldByStart(t, m)
		m.DiscardMetadata(hash)
		discarded = true
	}}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	_, err := m.StartDownloadFrom(hash, destination, nil, Origin{})

	if !discarded {
		t.Fatal("the folder check did not fail")
	}
	if got := uierr.Code(err); got != "download.destination_unavailable" {
		t.Fatalf("code = %q (%v), want download.destination_unavailable", got, err)
	}
	assertNothingLeftOfTheDiscarded(t, m, hash, source, true)
}

// A Start that succeeds is not undone by a discard that came during it: the
// download stays with its engine and its cached torrent, and nothing waits.
func TestADiscardDuringASucceedingStartLeavesTheDownload(t *testing.T) {
	m, _, hash, source := fetchedForDiscard(t)
	discardOnFirstAdd(t, m, hash, nil)

	d, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.wg.Wait()

	if got := m.List(); len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("downloads = %+v, want the started one", got)
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents in the client = %d, want the download's", got)
	}
	if !m.store.hasMetainfo(hash) {
		t.Fatal("the started download lost its cached torrent")
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d after the start", pending, reserved)
	}
	if _, err := m.FetchMetadata(source); !errors.Is(err, errDuplicateDownload) {
		t.Fatalf("fetching a started download = %v, want errDuplicateDownload", err)
	}
}
