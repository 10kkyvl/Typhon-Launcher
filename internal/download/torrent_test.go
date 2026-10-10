package download

import (
	"errors"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"testing"

	"typhon/internal/download/listenport"
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

// openTestClient is what the stand-ins for newClient share. A random port is
// free for TCP and can still be held in UDP by another process or sit in a
// range Windows excludes (listen udp4: bind: forbidden by its access
// permissions), so a client that fails to listen is tried again on a random
// port of its own choosing, as newClient does (port 0 hands out the next number
// in a row, which sits in the same excluded range), and only a client that
// fails all of them is an error. mk builds a fresh config for every attempt.
func openTestClient(mk func() (*torrent.ClientConfig, error)) (*torrent.Client, *torrent.ClientConfig, error) {
	for attempt := 0; ; attempt++ {
		tc, err := mk()
		if err != nil {
			return nil, nil, err
		}
		if attempt > 0 {
			if tc.ListenPort, err = listenport.Random(); err != nil {
				closeDefaultStorage(tc)
				return nil, nil, err
			}
		}
		cl, err := openTorrentClient(tc)
		if err == nil {
			return cl, tc, nil
		}
		closeDefaultStorage(tc)
		if !listenport.IsListenError(err) || attempt == listenport.Attempts {
			return nil, nil, err
		}
	}
}

func offlineClient(t *testing.T) *client {
	t.Helper()
	dir := t.TempDir()
	// Match production ownership: per-torrent storage must not clear the
	// shared completion map while another torrent is still hashing.
	completion := nonClosingCompletion{storage.NewMapPieceCompletion()}
	cl, tc, err := openTestClient(func() (*torrent.ClientConfig, error) {
		tc := clientConfig(settings.Defaults(), dir, 0, completion)
		tc.NoDHT = true
		tc.DisableTrackers = true
		tc.DisablePEX = true
		tc.NoDefaultPortForwarding = true
		return tc, nil
	})
	if err != nil {
		t.Fatalf("torrent client unavailable: %v", err)
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
