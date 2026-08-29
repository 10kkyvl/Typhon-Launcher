package lan

import (
	"bytes"
	"io"
	"testing"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// TestLanClientConfigHasNoPublicAnnounce guards the one fact the whole
// package's architecture depends on: anacrolix/torrent v1.61.0 never reads
// info.Private, so the only thing keeping a shared game's infohash off the
// public DHT is this config. A future "just reuse the download client"
// simplification would silently announce every LAN share to the world.
func TestLanClientConfigHasNoPublicAnnounce(t *testing.T) {
	cfg := newClientConfig(settings.Defaults(), t.TempDir(), 0)
	if !cfg.NoDHT {
		t.Error("NoDHT = false, want true")
	}
	if !cfg.DisableTrackers {
		t.Error("DisableTrackers = false, want true")
	}
	if !cfg.DisablePEX {
		t.Error("DisablePEX = false, want true")
	}
	if !cfg.NoDefaultPortForwarding {
		t.Error("NoDefaultPortForwarding = false, want true")
	}
}

// TestSpecCarriesNoTrackers is the same leak through the other door: even
// with the client locked down, a TorrentSpec built from a metainfo that
// happens to carry trackers or DHT nodes must have them stripped before
// AddTorrentSpec, or MergeSpec would still register them against the
// torrent.
func TestSpecCarriesNoTrackers(t *testing.T) {
	info := metainfo.Info{Name: "game", Length: 4, PieceLength: metainfo.ChoosePieceLength(4)}
	if err := info.GeneratePieces(func(fi metainfo.FileInfo) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("abcd"))), nil
	}); err != nil {
		t.Fatalf("generate pieces: %v", err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	mi := &metainfo.MetaInfo{
		InfoBytes: infoBytes,
		Announce:  "http://tracker.example/announce",
		AnnounceList: metainfo.AnnounceList{
			{"http://tracker.example/announce"},
		},
		Nodes: []metainfo.Node{"router.example:6881"},
	}

	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		t.Fatalf("spec from metainfo: %v", err)
	}
	if len(spec.Trackers) == 0 {
		t.Fatal("test setup broken: spec has no trackers to strip")
	}

	clearSpec(spec)

	if spec.Trackers != nil {
		t.Errorf("Trackers = %v, want nil", spec.Trackers)
	}
	if spec.Webseeds != nil {
		t.Errorf("Webseeds = %v, want nil", spec.Webseeds)
	}
	if spec.DhtNodes != nil {
		t.Errorf("DhtNodes = %v, want nil", spec.DhtNodes)
	}
	if spec.Sources != nil {
		t.Errorf("Sources = %v, want nil", spec.Sources)
	}
	if spec.PeerAddrs != nil {
		t.Errorf("PeerAddrs = %v, want nil", spec.PeerAddrs)
	}
}
