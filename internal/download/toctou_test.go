package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// startTogether blocks both goroutines behind a barrier so they are provably
// both waiting before either is released into the code under test, without
// any time.Sleep.
func startTogether(n int) (ready chan struct{}, start chan struct{}, wait func()) {
	ready = make(chan struct{}, n)
	start = make(chan struct{})
	return ready, start, func() {
		for i := 0; i < n; i++ {
			<-ready
		}
		close(start)
	}
}

func TestReserveHashIsExclusive(t *testing.T) {
	m := newTestManager(t, 1)
	const hash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	const n = 20
	ready, start, release := startTogether(n)
	results := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func() {
			ready <- struct{}{}
			<-start
			results <- m.reserveHash(hash)
		}()
	}
	release()

	successes := 0
	for i := 0; i < n; i++ {
		if <-results {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful reservations = %d, want 1", successes)
	}
}

// TestConcurrentAddTaskSameHashAddsOnce reproduces the tasks.go window from
// the audit: two goroutines calling AddTask for the same infohash raced
// through the old hashBusy-then-unlock-then-addMetainfo sequence with
// nothing stopping both from reaching cl.addMetainfo. Before the reservation
// fix this either let both succeed (two Downloads silently sharing one
// *torrent.Torrent, one leaked from the client's perspective) or leaked a
// live torrent nobody in the manager referenced. The metainfo is pre-cached
// so AddTask never blocks on network I/O, keeping the race entirely inside
// the manager's own locking.
func TestConcurrentAddTaskSameHashAddsOnce(t *testing.T) {
	mi, _ := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()

	m := newTestManager(t, 5)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}

	type outcome struct {
		d   Download
		err error
	}
	const n = 2
	ready, start, release := startTogether(n)
	results := make(chan outcome, n)
	for i := 0; i < n; i++ {
		dest := t.TempDir()
		go func(dest string) {
			ready <- struct{}{}
			<-start
			d, err := m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: dest})
			results <- outcome{d, err}
		}(dest)
	}
	release()

	var oks, fails int
	var loseErr error
	for i := 0; i < n; i++ {
		r := <-results
		if r.err == nil {
			oks++
			continue
		}
		fails++
		loseErr = r.err
	}
	if oks != 1 || fails != 1 {
		t.Fatalf("got %d ok, %d failed AddTask calls, want 1 and 1", oks, fails)
	}
	if !errors.Is(loseErr, errDuplicateTask) {
		t.Fatalf("losing AddTask error = %v, want errDuplicateTask", loseErr)
	}
	if got := len(m.List()); got != 1 {
		t.Fatalf("items tracked by manager = %d, want 1", got)
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents tracked by the real client = %d, want exactly 1 (a leak means the loser's add was not rejected)", got)
	}
}

// TestConcurrentFetchMetadataSameHashNeverDoublesUp reproduces the widest
// window from the audit: FetchMetadata's check-under-lock was followed by an
// unlocked cl.add plus up to metadataTimeout of waiting before the result
// was recorded in m.pending. A second concurrent call for the same hash
// could pass the same busy check and add its own competing torrent. The
// source here is a real .torrent file (not a magnet), so GotInfo resolves
// immediately from the spec itself without depending on network peers,
// keeping this test fast and hermetic.
func TestConcurrentFetchMetadataSameHashNeverDoublesUp(t *testing.T) {
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

	type outcome struct {
		info TorrentInfo
		err  error
	}
	const n = 2
	ready, start, release := startTogether(n)
	results := make(chan outcome, n)
	for i := 0; i < n; i++ {
		go func() {
			ready <- struct{}{}
			<-start
			info, err := m.FetchMetadata(torrentPath)
			results <- outcome{info, err}
		}()
	}
	release()

	var oks int
	for i := 0; i < n; i++ {
		r := <-results
		switch {
		case r.err == nil:
			oks++
			if r.info.InfoHash != hash {
				t.Fatalf("info hash = %s, want %s", r.info.InfoHash, hash)
			}
		case errors.Is(r.err, errHashBusy):
			// The loser hit the reservation before touching the client at
			// all; a legitimate, expected outcome of the race.
		default:
			t.Fatalf("unexpected FetchMetadata error: %v", r.err)
		}
	}
	// Two successes are a legitimate outcome, not a doubled add: the
	// metainfo comes from a local file, so GotInfo resolves at once and the
	// winner can reach m.pending before the loser even takes the lock, at
	// which point the loser is answered from m.pending instead of touching
	// the client (manager.go's "already pending" branch). What must never
	// happen is a second cl.add for the same hash — that is the leak the
	// reservation exists to prevent, and it shows up here as a torrent count
	// above one (a second liveTorrent whose storage nothing owns) or as an
	// errTorrentAlreadyAdded caught by the default branch above. The two
	// halves of the fix are each pinned deterministically elsewhere:
	// TestReserveHashIsExclusive for the reservation, and
	// TestAddRejectsSecondSpecForSameHash for the isNew check this relies on.
	if oks == 0 {
		t.Fatal("both concurrent FetchMetadata calls failed, want at least one to succeed")
	}
	if got := len(m.client.cl.Torrents()); got != 1 {
		t.Fatalf("torrents in the client = %d, want exactly 1 for one infohash", got)
	}

	m.DiscardMetadata(hash)
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the real client after discard = %d, want 0", got)
	}
}

// TestConcurrentAddTaskAndInspectReuseSameHash races two different manager
// entry points named separately in the audit (tasks.go's queue and
// reuse.go's InspectReuse) against the same infohash, closing the
// reuse.go:347->379 window: hashBusy used to run under its own lock, get
// released, and only then call cl.addMetainfo — the exact gap AddTask (or
// another InspectReuse call) could race through.
//
// InspectReuse always drops its own probe torrent before returning, success
// or not, so a plain start-together barrier cannot reliably force an
// overlap: on a slow enough scheduler InspectReuse can finish (and clean up
// after itself) before AddTask even begins, which is a legitimate
// non-conflicting outcome, not a race hit. The onProgress callback instead
// pins InspectReuse inside its reservation window (it runs synchronously
// from inside InspectReuse's own verify call) until AddTask has had its
// chance to race through, which is what makes this deterministic without
// any time.Sleep.
func TestConcurrentAddTaskAndInspectReuseSameHash(t *testing.T) {
	mi, dataDir := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()

	m := newTestManager(t, 5)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	reuseResult := make(chan error, 1)
	go func() {
		_, err := m.InspectReuse(context.Background(), ReuseRequest{InfoHash: hash, Path: dataDir}, func(VerifyProgress) {
			close(entered)
			<-release
		})
		reuseResult <- err
	}()

	<-entered // InspectReuse is now guaranteed to be holding the reservation.
	_, addErr := m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: t.TempDir()})
	close(release)
	reuseErr := <-reuseResult

	if !errors.Is(addErr, errDuplicateTask) {
		t.Fatalf("AddTask racing an in-flight InspectReuse = %v, want errDuplicateTask", addErr)
	}
	if reuseErr != nil {
		t.Fatalf("InspectReuse: %v", reuseErr)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the real client once InspectReuse finished = %d, want 0", got)
	}
}
