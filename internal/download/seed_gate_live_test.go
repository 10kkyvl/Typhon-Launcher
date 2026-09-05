package download

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// makeSeedData writes a real payload and returns its metainfo plus the
// directory the data lives in, so a live client can seed it in place.
func makeSeedData(t *testing.T, size int64) (*metainfo.MetaInfo, string) {
	t.Helper()
	dir := t.TempDir()
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 256 << 10}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatalf("build info: %v", err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes}
	return mi, dir
}

// runSeedGate wires a real seeding client to a real leeching client over
// loopback and reports what actually crossed the wire.
func runSeedGate(t *testing.T, allowUpload bool) (leecherGot int64, seederSent int64, total int64) {
	t.Helper()
	mi, dataDir := makeSeedData(t, 4<<20)

	seedCl := offlineClient(t)
	seed, err := seedCl.addMetainfo(mi, dataDir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("add seeder torrent: %v", err)
	}
	t.Cleanup(seed.drop)
	<-seed.t.GotInfo()
	total = seed.t.Length()

	// The seeder must actually hold the data before the gate means anything.
	select {
	case <-seed.t.Complete().On():
	case <-time.After(30 * time.Second):
		t.Fatalf("seeder holds %d/%d bytes, cannot test the gate", seed.t.BytesCompleted(), total)
	}

	// Exactly the two calls the manager makes for a completed download.
	seed.disallowDownload()
	if allowUpload {
		seed.allowUpload()
	} else {
		seed.disallowUpload()
	}

	leechCl := offlineClient(t)
	leech, err := leechCl.addMetainfo(mi, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("add leecher torrent: %v", err)
	}
	t.Cleanup(leech.drop)
	<-leech.t.GotInfo()
	leech.allowDownload()
	leech.setPriorities([]bool{true})
	if n := leech.t.AddClientPeer(seedCl.cl); n == 0 {
		t.Fatal("leecher could not be pointed at the seeder")
	}

	// Either the transfer finishes, or the window closes with the gate holding.
	select {
	case <-leech.t.Complete().On():
	case <-time.After(5 * time.Second):
	}
	return leech.t.BytesCompleted(), seedStats(seed).BytesWrittenData.Int64(), total
}

// TestSeedGateControlUploadsWhenAllowed is the positive control: without it,
// a zero-byte result in the test below would prove nothing about the gate.
func TestSeedGateControlUploadsWhenAllowed(t *testing.T) {
	got, sent, total := runSeedGate(t, true)
	t.Logf("allowUpload: leecher got %d/%d bytes, seeder sent %d", got, total, sent)
	if got != total {
		t.Fatalf("leecher got %d/%d bytes with upload allowed", got, total)
	}
	if sent < total {
		t.Fatalf("seeder sent %d bytes, want at least %d", sent, total)
	}
}

func TestSeedGateSendsNothingWhenDisallowed(t *testing.T) {
	got, sent, total := runSeedGate(t, false)
	t.Logf("disallowUpload: leecher got %d/%d bytes, seeder sent %d", got, total, sent)
	if got != 0 {
		t.Fatalf("leecher received %d bytes from a torrent with upload disallowed", got)
	}
	if sent != 0 {
		t.Fatalf("seeder sent %d bytes with upload disallowed", sent)
	}
}

// seedStats returns the seeder's connection stats by value so the counter's
// pointer method is addressable.
func seedStats(l *liveTorrent) *torrent.TorrentStats {
	st := l.t.Stats()
	return &st
}

// TestGatingLeavesTorrentInClientDropRemovesIt pins down why turning seeding
// off detaches instead of only gating: a gated torrent sends no data, but it
// is still registered in the client, which is what keeps it announcing itself
// to the tracker. Only drop takes it out.
func TestGatingLeavesTorrentInClientDropRemovesIt(t *testing.T) {
	mi, dataDir := makeSeedData(t, 1<<20)
	cl := offlineClient(t)
	lt, err := cl.addMetainfo(mi, dataDir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("add torrent: %v", err)
	}
	<-lt.t.GotInfo()
	hash := lt.t.InfoHash()

	lt.disallowUpload()
	if !inClient(cl, hash) {
		t.Fatal("gated torrent already left the client")
	}
	if lt.t.Seeding() {
		t.Fatal("gated torrent still reports seeding")
	}

	lt.drop()
	if inClient(cl, hash) {
		t.Fatal("dropped torrent is still registered in the client")
	}
}

func inClient(c *client, hash metainfo.Hash) bool {
	for _, t := range c.cl.Torrents() {
		if t.InfoHash() == hash {
			return true
		}
	}
	return false
}

// TestSeedToggleRoundTripOnLiveClient runs the full toggle cycle against a
// real torrent client: turning seeding off must take the torrent out of the
// client, and turning it back on must bring it back and actually upload
// again. The re-add goes through reseedLocked, so this also proves the
// metainfo the manager stored at add time is enough to reseed from.
func TestSeedToggleRoundTripOnLiveClient(t *testing.T) {
	mi, dataDir := makeSeedData(t, 1<<20)
	hash := mi.HashInfoBytes()

	cfg := settings.Defaults()
	cfg.SeedAfterDownload = true
	m, svc := newManagerWithSettings(t, cfg)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash.HexString(), mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}

	d := m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	d.InfoHash = hash.HexString()
	d.Destination = dataDir
	d.InPlace = true
	d.Seeding = true
	d.Files = []FileState{{Path: "payload.bin", Size: 1 << 20, Selected: true}}
	d.Total = 1 << 20
	m.mu.Unlock()

	lt, err := m.client.addMetainfo(mi, dataDir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("attach seeding torrent: %v", err)
	}
	lt.allowUpload()
	m.mu.Lock()
	m.engines["a"] = lt
	m.mu.Unlock()

	cfg.SeedAfterDownload = false
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	m.applySettings(cfg)

	waitUntil(t, "torrent to leave the client", func() bool { return !inClient(m.client, hash) })
	if seedingOf(t, m, "a") {
		t.Fatal("seeding still reported after the toggle went off")
	}

	cfg.SeedAfterDownload = true
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	m.applySettings(cfg)

	waitUntil(t, "torrent to come back", func() bool { return inClient(m.client, hash) })
	waitUntil(t, "seeding to resume", func() bool { return seedingOf(t, m, "a") })
	m.mu.Lock()
	back := m.engines["a"]
	m.mu.Unlock()
	if back == nil {
		t.Fatal("engine not reattached")
	}
	t.Cleanup(back.drop)
}
