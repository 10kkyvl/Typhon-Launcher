package download

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

// fetchedTorrent is a manager whose add dialog has fetched the metadata of a
// torrent file and waits for Start.
func fetchedTorrent(t *testing.T) (*Manager, string) {
	t.Helper()
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 1)
	if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	return m, hash
}

func assertWaitingForStart(t *testing.T, m *Manager) {
	t.Helper()
	if got := len(m.List()); got != 0 {
		t.Fatalf("a failed start left %d downloads", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 1 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d: the fetched torrent must wait for the next Start", pending, reserved)
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents in the client = %d, want only the one waiting for Start", got)
	}
}

func assertStartsWithoutAFetch(t *testing.T, m *Manager, hash string) {
	t.Helper()
	d, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})
	if err != nil {
		t.Fatalf("second Start without a new fetch: %v", err)
	}
	if d.Total != 60000 {
		t.Fatalf("download = %+v", d)
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents in the client = %d, want only the download", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d after the start", pending, reserved)
	}
	m.wg.Wait()
}

func assertMustFetchAgain(t *testing.T, m *Manager, err error, cause error) {
	t.Helper()
	if got := uierr.Code(err); got != "download.metadata_required" {
		t.Fatalf("code = %q (%v), want download.metadata_required: the window has to fetch again", got, err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error %v lost its cause %v", err, cause)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("a failed start left %d downloads", got)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d: nothing may wait for a Start that cannot work", pending, reserved)
	}
}

func TestStartHandsTheTorrentBackWhenTheListIsNotSaved(t *testing.T) {
	m, hash := fetchedTorrent(t)
	breakDownloadsFile(t, m)

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	if got := uierr.Code(err); got != "download.save_failed" {
		t.Fatalf("code = %q (%v), want download.save_failed", got, err)
	}
	assertWaitingForStart(t, m)

	if err := os.Remove(filepath.Join(m.store.dir, "downloads.json")); err != nil {
		t.Fatal(err)
	}
	assertStartsWithoutAFetch(t, m, hash)
}

func TestStartHandsTheTorrentBackWhenItsFileIsNotCached(t *testing.T) {
	m, hash := fetchedTorrent(t)
	cached := m.store.metainfoPath(hash)
	if err := os.Remove(cached); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory where the cached torrent goes makes its write fail.
	if err := os.MkdirAll(filepath.Join(cached, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	if !errors.Is(err, errMetainfoNotSaved) {
		t.Fatalf("error = %v, want errMetainfoNotSaved", err)
	}
	assertWaitingForStart(t, m)

	if err := os.RemoveAll(cached); err != nil {
		t.Fatal(err)
	}
	assertStartsWithoutAFetch(t, m, hash)
	if !m.store.hasMetainfo(hash) {
		t.Fatal("the second Start did not cache the torrent")
	}
}

func TestStartOnAStoppedClientAsksForANewFetch(t *testing.T) {
	m, hash := fetchedTorrent(t)
	m.client.halt()

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	assertMustFetchAgain(t, m, err, errNetworkDown)
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the stopped client = %d", got)
	}
}

func TestStartAcrossAClientChangeAsksForANewFetch(t *testing.T) {
	m, hash := fetchedTorrent(t)
	old, next := m.client, offlineClient(t)
	// The change lands while Start adds the torrent at its destination, the
	// window in which Start holds no lock.
	old.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		m.replaceClient(next)
		return tiers, nil
	}

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	assertMustFetchAgain(t, m, err, errNetworkDown)
	if got := len(old.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the replaced client = %d", got)
	}
	if got := len(next.cl.Torrents()); got != 0 {
		t.Fatalf("torrents in the new client = %d", got)
	}
}

func TestStartAsksForANewFetchWhenTheClientChangesWhileTheTorrentIsHandedBack(t *testing.T) {
	m, hash := fetchedTorrent(t)
	breakDownloadsFile(t, m)
	old, next := m.client, offlineClient(t)
	adds := 0
	// The first add is Start's at the destination; the second re-adds the
	// fetched torrent to wait for the next Start, and the change lands there.
	old.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		adds++
		if adds == 2 {
			m.replaceClient(next)
		}
		return tiers, nil
	}

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	if adds != 2 {
		t.Fatalf("adds = %d, want the destination add and the hand-back add", adds)
	}
	assertMustFetchAgain(t, m, err, errSaveFailed)
	if got := len(old.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the replaced client = %d", got)
	}
	if got := len(next.cl.Torrents()); got != 0 {
		t.Fatalf("torrents in the new client = %d", got)
	}
}

// A new fetch cannot help when the list was not read: every Start is refused
// until the file is dealt with, so the refusal's own code comes first even
// when nothing is left waiting.
func TestStartRefusedForAnUnreadListSaysSoWhenNothingWaits(t *testing.T) {
	m, hash := fetchedTorrent(t)
	loadErr := loseTheList(t, m)
	old, next := m.client, offlineClient(t)
	adds := 0
	old.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		adds++
		if adds == 2 {
			m.replaceClient(next)
		}
		return tiers, nil
	}

	_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})

	if adds != 2 {
		t.Fatalf("adds = %d, want the destination add and the hand-back add", adds)
	}
	if got := uierr.Code(err); got != "download.state_unread" {
		t.Fatalf("code = %q (%v), want download.state_unread: fetching again would be refused the same way", got, err)
	}
	if !errors.Is(err, errStateUnread) || !errors.Is(err, loadErr) {
		t.Fatalf("error %v lost why the list was not read", err)
	}
	if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
		t.Fatalf("pending = %d, reserved = %d", pending, reserved)
	}
}
