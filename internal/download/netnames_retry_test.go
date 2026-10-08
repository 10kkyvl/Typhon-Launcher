package download

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// tickGate stands in for the backoff between two tries: a try only happens
// when the test says so, and the test can tell how often a try waited.
type tickGate struct {
	mu       sync.Mutex
	attempts []int
	ch       chan time.Time
}

func newTickGate() *tickGate { return &tickGate{ch: make(chan time.Time)} }

func (g *tickGate) after(attempt int) <-chan time.Time {
	g.mu.Lock()
	g.attempts = append(g.attempts, attempt)
	g.mu.Unlock()
	return g.ch
}

func (g *tickGate) waited() []int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]int(nil), g.attempts...)
}

func (g *tickGate) fire(t *testing.T) {
	t.Helper()
	select {
	case g.ch <- time.Now():
	case <-time.After(5 * time.Second):
		t.Fatal("no retry is waiting for its turn")
	}
}

func (g *tickGate) waitFor(t *testing.T, n int) {
	t.Helper()
	waitUntil(t, "a retry to wait for its turn", func() bool { return len(g.waited()) >= n })
}

const retryMagnet = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x" +
	"&tr=udp%3A%2F%2Ftracker.example%3A6969%2Fannounce&tr=http%3A%2F%2Fh.example%2Fa"

// retryClient is an interface-mode client whose tracker names go through dns.
// The name server only knows other.example at first, so tracker.example fails.
func retryClient(t *testing.T, dns *fakeDNS) (*client, *tickGate) {
	t.Helper()
	_, names := boundResolver(t, dns, "127.0.0.3")
	gate := newTickGate()
	later := newRetrier(t.Context())
	later.after = gate.after
	names.later = later
	names.seen = newHostLog()
	cl := offlineClient(t)
	cl.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		return names.resolveUDPTrackers(t.Context(), tiers)
	}
	cl.retryTrackers = names.retryLost
	cl.later = later
	return cl, gate
}

func addRetryMagnet(t *testing.T, cl *client) *liveTorrent {
	t.Helper()
	lt, err := cl.addMagnet(retryMagnet, cl.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("addMagnet: %v", err)
	}
	t.Cleanup(lt.drop)
	return lt
}

func TestUDPTrackerWhoseNameDidNotResolveComesBackLater(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	cl, gate := retryClient(t, dns)
	lt := addRetryMagnet(t, cl)

	if got := strings.Join(announced(lt), " "); got != "http://h.example/a" {
		t.Fatalf("trackers of the engine = %q, want only the http one while the name does not resolve", got)
	}
	gate.waitFor(t, 1)
	dns.allow("tracker.example")
	gate.fire(t)

	waitUntil(t, "the tracker to come back", func() bool { return slices.Contains(announced(lt), "udp://203.0.113.9:6969/announce") })
	var stored []string
	for _, tier := range lt.metainfo().AnnounceList {
		stored = append(stored, tier...)
	}
	if !slices.Contains(stored, "udp://tracker.example:6969/announce") {
		t.Fatalf("stored trackers = %v, want the original name and not the address", stored)
	}
	done := make(chan struct{})
	go func() {
		cl.later.wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry goes on after every tracker is back")
	}
}

func TestRetryGoesOnWhileTheNameStaysUnresolved(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	cl, gate := retryClient(t, dns)
	lt := addRetryMagnet(t, cl)

	for i := 1; i <= 3; i++ {
		gate.waitFor(t, i)
		gate.fire(t)
	}
	gate.waitFor(t, 4)
	if slices.Contains(announced(lt), "udp://203.0.113.9:6969/announce") {
		t.Fatal("the tracker appeared although its name never resolved")
	}
	dns.allow("tracker.example")
	gate.fire(t)
	waitUntil(t, "the tracker to come back", func() bool { return slices.Contains(announced(lt), "udp://203.0.113.9:6969/announce") })
	if got := gate.waited(); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Fatalf("attempts waited for = %v, want a growing count so that the backoff grows", got)
	}
}

func TestRetryEndsWithItsTorrentAndWithItsClient(t *testing.T) {
	cases := []struct {
		name string
		end  func(*client, *liveTorrent)
	}{
		{"torrent dropped", func(_ *client, lt *liveTorrent) { lt.drop() }},
		{"client closed", func(cl *client, _ *liveTorrent) { cl.close() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
			cl, gate := retryClient(t, dns)
			lt := addRetryMagnet(t, cl)
			gate.waitFor(t, 1)

			c.end(cl, lt)
			done := make(chan struct{})
			go func() {
				cl.later.wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the retry goroutine outlived its owner")
			}
		})
	}
}

// linesMentioning counts the records, not the occurrences: the error text of a
// failed lookup names the host again.
func linesMentioning(log, host string) int {
	n := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, host) {
			n++
		}
	}
	return n
}

func TestFailedLookupIsLoggedOncePerName(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "tracker.example")
	cases := []struct {
		name string
		ask  func(nameResolver)
	}{
		{"tracker list", func(n nameResolver) {
			n.resolveUDPTrackers(t.Context(), [][]string{{"udp://gone.example:80", "udp://tracker.example:80"}})
		}},
		{"dht bootstrap lookup", func(n nameResolver) { n.lookup(t.Context(), "gone.example") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, names := boundResolver(t, dns, "127.0.0.3")
			var buf bytes.Buffer
			names.log = slog.New(slog.NewTextHandler(&buf, nil))
			names.seen = newHostLog()

			for range 3 {
				c.ask(names)
			}
			out := buf.String()
			if n := linesMentioning(out, "gone.example"); n != 1 {
				t.Fatalf("gone.example was logged %d times in\n%s", n, out)
			}
			if !strings.Contains(out, "level=WARN") {
				t.Fatalf("a name that does not resolve is not a debug matter:\n%s", out)
			}
			if strings.Contains(out, "tracker.example") {
				t.Fatalf("a name that resolved was logged:\n%s", out)
			}
			names.lookup(t.Context(), "also-gone.example")
			if n := linesMentioning(buf.String(), "also-gone.example"); n != 1 {
				t.Fatalf("a second name was logged %d times", n)
			}
		})
	}
}

func TestRetryGivesUpOnANameThatDoesNotExist(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	cl, gate := retryClient(t, dns)
	addRetryMagnet(t, cl)
	done := make(chan struct{})
	go func() {
		cl.later.wait()
		close(done)
	}()
	ended := func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}

	for i := 1; ; i++ {
		if i > 20 {
			t.Fatal("still retrying every few minutes after 20 tries for a name that does not exist")
		}
		waitUntil(t, "the next try to wait for its turn or the retry to end", func() bool { return ended() || len(gate.waited()) >= i })
		if ended() {
			return
		}
		gate.fire(t)
	}
}

func TestRecoverKeepsTryingWhileTheNameServerIsDown(t *testing.T) {
	down := &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("name server unreachable")
	}}
	names := nameResolver{r: down, allow4: true, seen: newHostLog()}
	lost := []lostTracker{{u: mustURL(t, "udp://tracker.example:6969/announce")}}
	for range 20 {
		lost = names.recover(t.Context(), nil, lost)
	}
	if len(lost) != 1 {
		t.Fatalf("lost = %v: a name server that is not up yet says nothing about the name", lost)
	}
}

func TestRecoverDropsANameTheServerSaysDoesNotExist(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	_, names := boundResolver(t, dns, "127.0.0.3")
	names.seen = newHostLog()
	lost := []lostTracker{{u: mustURL(t, "udp://gone.example:6969/announce")}}
	for i := 0; i < 20 && len(lost) > 0; i++ {
		lost = names.recover(t.Context(), nil, lost)
	}
	if len(lost) != 0 {
		t.Fatalf("lost = %v after 20 answers that the name does not exist", lost)
	}
}

func TestSecondRetryOfTheSameTorrentJoinsTheFirst(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	cl, gate := retryClient(t, dns)
	lt := addRetryMagnet(t, cl)
	gate.waitFor(t, 1)

	_, same := cl.filterTrackers([][]string{{"udp://tracker.example:6969/announce"}})
	cl.retryTrackers(lt.t, same)
	_, other := cl.filterTrackers([][]string{{"udp://second.example:80"}})
	cl.retryTrackers(lt.t, other)

	dns.allow("tracker.example")
	dns.allow("second.example")
	gate.fire(t)
	waitUntil(t, "both trackers to come back on one try", func() bool {
		got := announced(lt)
		return slices.Contains(got, "udp://203.0.113.9:6969/announce") && slices.Contains(got, "udp://203.0.113.9:80")
	})
	done := make(chan struct{})
	go func() {
		cl.later.wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a second goroutine for the same torrent is still waiting for its turn")
	}
}

func TestRecoveredTrackerGoesBackToItsOwnTier(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "other.example")
	cl, gate := retryClient(t, dns)
	mi, dir := makeSeedData(t, 256<<10)
	mi.AnnounceList = [][]string{{"udp://tracker.example:6969/announce"}, {"http://h.example/a"}}
	lt, err := cl.addMetainfo(mi, dir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("addMetainfo: %v", err)
	}
	t.Cleanup(lt.drop)
	gate.waitFor(t, 1)
	dns.allow("tracker.example")
	gate.fire(t)

	want := [][]string{{"udp://203.0.113.9:6969/announce"}, {"http://h.example/a"}}
	var got [][]string
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("announce list = %q", got)
		}
	})
	waitUntil(t, "the tracker to come back to the first tier", func() bool {
		got = lt.t.Metainfo().AnnounceList
		return slices.EqualFunc(got, want, slices.Equal[[]string])
	})
}

func TestInterfaceConfigRetriesUnresolvedTrackers(t *testing.T) {
	tc := testConfig(t)
	plan := ifacePlan("10.8.0.2", "")
	plan.dns = "10.8.0.1"
	attach, err := applyNetwork(t.Context(), tc, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(attach.later.stop)
	if attach.retry == nil || attach.later == nil {
		t.Fatal("trackers whose name does not resolve need a way back: retry and its owner must be wired")
	}
	proxy, err := applyNetwork(t.Context(), testConfig(t), proxyPlan("socks5"))
	if err != nil {
		t.Fatal(err)
	}
	if proxy.retry != nil || proxy.later != nil {
		t.Fatal("a proxy client resolves no names of its own")
	}
}

// TestRestoredMagnetStoresTheTrackersItWasGiven: the engine was handed only
// the trackers that resolved, and what goes to disk has to be the list as it
// came in, or the next client, behind another adapter, loses the rest.
func TestRestoredMagnetStoresTheTrackersItWasGiven(t *testing.T) {
	mi, dataDir := makeSeedData(t, 256<<10)
	hash := mi.HashInfoBytes().HexString()
	uri := "magnet:?xt=urn:btih:" + hash + "&dn=x" +
		"&tr=udp%3A%2F%2Ftracker.example%3A6969%2Fannounce&tr=udp%3A%2F%2Fgone.example%3A80"
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "tracker.example")
	_, names := boundResolver(t, dns, "127.0.0.3")

	m := newTestManager(t, 1)
	d := m.addTestItem("a", StatusQueued)
	m.mu.Lock()
	d.InfoHash = hash
	m.mu.Unlock()
	cl := offlineClient(t)
	cl.filterTrackers = func(tiers [][]string) ([][]string, []lostTracker) {
		return names.resolveUDPTrackers(t.Context(), tiers)
	}

	infoSet := make(chan error, 1)
	go func() {
		for {
			if ts := cl.cl.Torrents(); len(ts) == 1 {
				infoSet <- ts[0].SetInfoBytes(mi.InfoBytes)
				return
			}
			select {
			case <-t.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()

	lt, err := m.reattach(t.Context(), cl, restoreJob{id: "a", infoHash: hash, source: uri, dest: dataDir})
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	t.Cleanup(lt.drop)
	if err := <-infoSet; err != nil {
		t.Fatalf("hand the metadata to the torrent: %v", err)
	}

	stored, err := m.store.loadMetainfo(hash)
	if err != nil {
		t.Fatalf("load stored metainfo: %v", err)
	}
	var flat []string
	for _, tier := range stored.AnnounceList {
		flat = append(flat, tier...)
	}
	if got := strings.Join(flat, " "); got != "udp://tracker.example:6969/announce udp://gone.example:80" {
		t.Fatalf("stored trackers = %q, want the list as it came in", got)
	}
}
