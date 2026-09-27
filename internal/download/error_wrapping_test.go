package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent"
)

// These tests force AddTask/FetchMetadata/StartDownloadFrom down their
// cl.add(Metainfo) failure branch by pre-adding the same infohash directly
// to the real client, bypassing the manager's own bookkeeping. That makes
// Client.AddTorrentSpec report isNew=false and client.add fail with
// errTorrentAlreadyAdded (torrent.go), which is exactly the kind of
// distinguishable cause bug #4 asks not to collapse into one generic
// "add failed" sentinel: errors.Is must still find it under the manager's
// wrapping.

func TestAddTaskWrapsUnderlyingAddFailure(t *testing.T) {
	mi, _ := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()

	m := newTestManager(t, 5)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}

	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	pre, err := m.client.add(spec, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("pre-add: %v", err)
	}
	t.Cleanup(pre.drop)

	_, err = m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: t.TempDir()})
	if err == nil {
		t.Fatal("AddTask succeeded despite the infohash already being tracked by the client")
	}
	if !errors.Is(err, errTorrentAlreadyAdded) {
		t.Fatalf("AddTask error = %v, want it to wrap errTorrentAlreadyAdded", err)
	}
}

func TestFetchMetadataWrapsUnderlyingAddFailure(t *testing.T) {
	mi, _ := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()

	torrentPath := filepath.Join(t.TempDir(), "x.torrent")
	f, err := os.Create(torrentPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := mi.Write(f); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	m := newTestManager(t, 5)
	m.client = offlineClient(t)

	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	pre, err := m.client.add(spec, m.client.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("pre-add: %v", err)
	}
	t.Cleanup(pre.drop)

	_, err = m.FetchMetadata(torrentPath)
	if err == nil {
		t.Fatal("FetchMetadata succeeded despite the infohash already being tracked by the client")
	}
	if !errors.Is(err, errAddTorrentFailed) {
		t.Fatalf("FetchMetadata error = %v, want it to wrap errAddTorrentFailed", err)
	}
	if !errors.Is(err, errTorrentAlreadyAdded) {
		t.Fatalf("FetchMetadata error = %v, want the underlying cause to survive the wrapping", err)
	}
	if got := hash; got != mi.HashInfoBytes().HexString() {
		t.Fatalf("sanity: unexpected hash mismatch %s", got)
	}
}

// manager.go's StartDownloadFrom wraps its own cl.addMetainfo failure with
// the exact same "%w: %w", errAddTorrentFailed pattern as FetchMetadata
// above (mechanically identical, verified by inspection), but there is no
// practical, non-flaky way to drive a real errTorrentAlreadyAdded into that
// specific branch in a test: StartDownloadFrom now reserves the infohash for
// the entire span from taking p out of m.pending to either a committed
// engine or a released reservation (see StartDownloadFrom's own comment in
// manager.go), which is exactly the fifth window this fix closes beyond the
// four the audit named. Constructing a conflicting add would require
// injecting it into the single synchronous gap between p.torrent.drop() and
// cl.addMetainfo() inside that function, which has no channel or callback to
// synchronize on — the only lever left is raw goroutine-scheduling luck,
// which would make the test flaky rather than a real regression check.
