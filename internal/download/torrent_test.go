package download

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"testing"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

func TestIsSafeTorrentPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"game/data/pak0.pak", true},
		{"setup.exe", true},
		{"", false},
		{"   ", false},
		{"../x", false},
		{`C:\x`, false},
		{"a/../../b", false},
		{"a//b", false},
		{"./b", false},
		{`dir\file`, false},
	}
	for _, c := range cases {
		if got := isSafeTorrentPath(c.path); got != c.want {
			t.Fatalf("isSafeTorrentPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if runtime.GOOS == "windows" && isSafeTorrentPath("con") {
		t.Fatal(`isSafeTorrentPath("con") = true, want false`)
	}
}

func TestLimiterBurst(t *testing.T) {
	if got := limiterBurst(1024); got != minLimiterBurst {
		t.Fatalf("burst = %d, want %d", got, minLimiterBurst)
	}
	if got := limiterBurst(4 << 20); got != 4<<20 {
		t.Fatalf("burst = %d, want %d", got, 4<<20)
	}
}

func TestIsListenError(t *testing.T) {
	bindErr := &net.OpError{
		Op:  "listen",
		Net: "udp4",
		Err: errors.New("Only one usage of each socket address is normally permitted."),
	}
	if !isListenError(bindErr) {
		t.Fatal("net.OpError not recognised as a listen failure")
	}
	if !isListenError(fmt.Errorf("wrapped: %w", bindErr)) {
		t.Fatal("wrapped net.OpError not recognised")
	}
	if !isListenError(errors.New("listen udp4 :42815: bind: address already in use")) {
		t.Fatal("plain bind error not recognised")
	}
	if isListenError(errors.New("не удалось прочитать torrent-файл")) {
		t.Fatal("unrelated error treated as a listen failure")
	}
}

func TestApplyLimit(t *testing.T) {
	l := newLimiter(0)
	if l.Limit() != rate.Inf {
		t.Fatalf("limit = %v, want Inf", l.Limit())
	}
	applyLimit(l, 2<<20)
	if l.Limit() != rate.Limit(2<<20) || l.Burst() != 2<<20 {
		t.Fatalf("limit = %v burst = %d", l.Limit(), l.Burst())
	}
	applyLimit(l, 0)
	if l.Limit() != rate.Inf {
		t.Fatalf("limit = %v, want Inf", l.Limit())
	}
}

func offlineClient(t *testing.T) *client {
	t.Helper()
	dir := t.TempDir()
	completion := storage.NewMapPieceCompletion()
	tc := clientConfig(settings.Defaults(), dir, 0, completion)
	tc.NoDHT = true
	tc.DisableTrackers = true
	tc.DisablePEX = true
	tc.NoDefaultPortForwarding = true
	cl, err := torrent.NewClient(tc)
	if err != nil {
		closeDefaultStorage(tc)
		t.Skipf("torrent client unavailable: %v", err)
	}
	c := &client{cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, metaDir: dir, completion: completion}
	t.Cleanup(c.close)
	return c
}

func trackerTiers(spec *torrent.TorrentSpec) []string {
	var out []string
	for _, tier := range spec.Trackers {
		for _, url := range tier {
			if strings.TrimSpace(url) != "" {
				out = append(out, url)
			}
		}
	}
	return out
}

func announced(lt *liveTorrent) []string {
	var out []string
	for _, tier := range lt.t.Metainfo().AnnounceList {
		for _, url := range tier {
			if strings.TrimSpace(url) != "" {
				out = append(out, url)
			}
		}
	}
	return out
}

func TestAddDoesNotInjectTrackers(t *testing.T) {
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=Startup+Panic"
	spec, err := magnetSpec(uri)
	if err != nil {
		t.Fatalf("magnetSpec: %v", err)
	}
	if got := trackerTiers(spec); len(got) != 0 {
		t.Fatalf("magnet already has trackers: %v", got)
	}

	cl := offlineClient(t)
	lt, err := cl.add(spec, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Cleanup(lt.drop)

	if got := trackerTiers(spec); len(got) != 0 {
		t.Fatalf("spec trackers = %v, want none", got)
	}
	if got := announced(lt); len(got) != 0 {
		t.Fatalf("announce list = %v, want none", got)
	}
}

func TestAddKeepsMagnetTrackers(t *testing.T) {
	const tracker = "udp://tracker.example:80/announce"
	uri := "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&tr=" + url.QueryEscape(tracker)
	spec, err := magnetSpec(uri)
	if err != nil {
		t.Fatalf("magnetSpec: %v", err)
	}

	cl := offlineClient(t)
	lt, err := cl.add(spec, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Cleanup(lt.drop)

	if got := trackerTiers(spec); !slices.Equal(got, []string{tracker}) {
		t.Fatalf("spec trackers = %v, want %v", got, []string{tracker})
	}
	if got := announced(lt); !slices.Equal(got, []string{tracker}) {
		t.Fatalf("announce list = %v, want %v", got, []string{tracker})
	}
}

func TestAddStartsWithUploadDisallowed(t *testing.T) {
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=Startup+Panic"
	spec, err := magnetSpec(uri)
	if err != nil {
		t.Fatalf("magnetSpec: %v", err)
	}
	cl := offlineClient(t)
	lt, err := cl.add(spec, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Cleanup(lt.drop)

	if lt.t.Seeding() {
		t.Fatal("fresh torrent reports seeding before the upload setting is applied")
	}
}

// TestAddRejectsSecondSpecForSameHash is the deterministic, single-goroutine
// counterpart to the concurrency tests in toctou_test.go: it proves add's
// own defence in depth (checking AddTorrentSpec's "new" return) works even
// with no race involved. Client.AddTorrentSpec merges a second spec for an
// already-tracked infohash into the first *torrent.Torrent instead of
// erroring, and MergeSpec documents that it ignores the second spec's
// Storage — silently returning a *liveTorrent built from that ignored
// storage would make the caller believe its destination was in effect when
// it never was.
func TestAddRejectsSecondSpecForSameHash(t *testing.T) {
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=Startup+Panic"
	cl := offlineClient(t)

	spec1, err := magnetSpec(uri)
	if err != nil {
		t.Fatalf("magnetSpec: %v", err)
	}
	firstDest := t.TempDir()
	lt1, err := cl.add(spec1, firstDest, storageOpts{})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	t.Cleanup(lt1.drop)

	spec2, err := magnetSpec(uri)
	if err != nil {
		t.Fatalf("magnetSpec: %v", err)
	}
	secondDest := t.TempDir()
	lt2, err := cl.add(spec2, secondDest, storageOpts{})
	if err == nil {
		t.Cleanup(lt2.drop)
		t.Fatal("second add for the same infohash succeeded, want errTorrentAlreadyAdded")
	}
	if !errors.Is(err, errTorrentAlreadyAdded) {
		t.Fatalf("second add error = %v, want errTorrentAlreadyAdded", err)
	}
	if lt2 != nil {
		t.Fatal("second add returned a non-nil *liveTorrent alongside its error")
	}

	// The original torrent must be unaffected: still exactly one torrent in
	// the client, still the one built with the first destination.
	torrents := cl.cl.Torrents()
	if len(torrents) != 1 {
		t.Fatalf("torrents tracked by client = %d, want 1", len(torrents))
	}
	if torrents[0] != lt1.t {
		t.Fatal("the tracked torrent is not the first liveTorrent's")
	}
}
