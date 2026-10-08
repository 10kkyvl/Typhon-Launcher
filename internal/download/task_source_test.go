package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAddTaskResolvesItsMetadataFromTheSource(t *testing.T) {
	const hash = "a748597437835a2fd0d2e06f8edd86fee316a84d"
	garbage := filepath.Join(t.TempDir(), "broken.torrent")
	if err := os.WriteFile(garbage, []byte("not a torrent"), 0o644); err != nil {
		t.Fatal(err)
	}
	live, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	gone, stop := context.WithCancel(context.Background())
	stop()

	cases := []struct {
		name      string
		source    func(t *testing.T) string
		cacheHash func(t *testing.T, m *Manager) string
		ctx       context.Context
		proxyOnly bool
		want      error
		anyErr    bool
	}{
		{"a torrent file", func(t *testing.T) string {
			mi, _ := threeFileTorrent(t)
			return writeTorrentFile(t, mi)
		}, nil, live, false, nil, false},
		{"the cached copy wins over a source that is gone", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "gone.torrent")
		}, func(t *testing.T, m *Manager) string {
			mi, _ := threeFileTorrent(t)
			h := mi.HashInfoBytes().HexString()
			if err := m.store.saveMetainfo(h, mi); err != nil {
				t.Fatal(err)
			}
			return h
		}, live, false, nil, false},
		{"a file that is not a torrent", func(*testing.T) string { return garbage }, nil, live, false, nil, true},
		{"a malformed magnet", func(*testing.T) string { return "magnet:?xt=urn:btih:zzzz" }, nil, live, false, nil, true},
		{"a magnet nobody answers", func(*testing.T) string { return "magnet:?xt=urn:btih:" + hash }, nil, live, false, errNoMetadata, false},
		{"a magnet nobody answers through a proxy", func(*testing.T) string { return "magnet:?xt=urn:btih:" + hash }, nil, live, true, errNoMetadataProxy, false},
		{"a caller that has given up", func(*testing.T) string { return "magnet:?xt=urn:btih:" + hash }, nil, gone, false, context.Canceled, false},
		{"nothing to go by", func(*testing.T) string { return "  " }, nil, live, false, errNoMetadata, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := metadataTimeout
			metadataTimeout = 20 * time.Millisecond
			t.Cleanup(func() { metadataTimeout = old })
			m := managerWithClient(t, 1)
			m.client.httpTrackersOnly = c.proxyOnly
			req := AddRequest{Source: c.source(t), Destination: t.TempDir()}
			if c.cacheHash != nil {
				req.InfoHash = c.cacheHash(t, m)
			}

			d, err := m.AddTask(c.ctx, req)

			wantOK := c.want == nil && !c.anyErr
			switch {
			case wantOK && err != nil:
				t.Fatalf("AddTask: %v", err)
			case !wantOK && err == nil:
				t.Fatal("the task was accepted")
			case c.want != nil && !errors.Is(err, c.want):
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if wantOK {
				if d.Source != req.Source || !m.store.hasMetainfo(d.InfoHash) {
					t.Fatalf("task = %+v, cached = %v", d, m.store.hasMetainfo(d.InfoHash))
				}
				return
			}
			if got := len(m.List()); got != 0 {
				t.Fatalf("downloads = %d", got)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("a refused task left %d torrents in the client", got)
			}
			if _, reserved := m.bookkeeping(); reserved != 0 {
				t.Fatalf("reserved = %d", reserved)
			}
		})
	}
}

func TestStartDownloadWithoutAnOriginIsAPlainRelease(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	hash := mi.HashInfoBytes().HexString()
	m := managerWithClient(t, 1)
	if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
		t.Fatal(err)
	}

	d, err := m.StartDownload(hash, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if d.Origin.Purpose != PurposeRelease || d.Origin.GameID != "" || d.Origin.AutoInstall != nil {
		t.Fatalf("origin = %+v", d.Origin)
	}
	if got := m.ByOrigin("", PurposeRelease); len(got) != 1 {
		t.Fatalf("ByOrigin(release) = %+v", got)
	}
	m.wg.Wait()
}
