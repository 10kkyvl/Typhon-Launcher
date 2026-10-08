package download

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

func TestRouteBackBringsParkedDownloadsAndSkipsFailedOnes(t *testing.T) {
	modes := []struct {
		name   string
		mutate func(*settings.Settings)
		up     func(r *netRig)
		down   func(r *netRig)
		code   string
	}{
		{"interface", viaInterface,
			func(r *netRig) { r.net.set(vpnIface("10.8.0.2")) },
			func(r *netRig) { r.net.set() },
			"download.net_interface_missing"},
		{"proxy", viaProxy,
			func(r *netRig) { r.net.setProbe(nil) },
			func(r *netRig) { r.net.setProbe(errors.New("connection refused")) },
			"download.proxy_unreachable"},
	}
	for _, c := range modes {
		t.Run(c.name, func(t *testing.T) {
			r := newNetRig(t, func(s *settings.Settings) { c.mutate(s); s.SeedAfterDownload = true })
			log := recordEmits(t)
			c.up(r)
			r.reconcile(t)
			first := r.client()
			if first == nil || r.builds.built() != 1 {
				t.Fatalf("no client to start from: %+v", r.state())
			}

			dl, dlMI := realDownload(t, r.m, "dl", StatusQueued, false)
			seed, seedMI := realDownload(t, r.m, "seed", StatusCompleted, true)
			realDownload(t, r.m, "failed", StatusFailed, false)
			setItem(r.m, "failed", func(d *Download) { d.Error = "disk full" })
			attachReal(t, r.m, first, "dl", dlMI, dl.Destination)
			attachReal(t, r.m, first, "seed", seedMI, seed.Destination)
			r.m.mu.Lock()
			r.m.engines["seed"].allowUpload()
			r.m.schedule()
			r.m.mu.Unlock()
			assertStatuses(t, r.m, map[string]Status{"dl": StatusDownloading, "seed": StatusCompleted, "failed": StatusFailed})

			c.down(r)
			for range 3 {
				r.reconcile(t)
			}

			st := r.state()
			if st.State != NetworkDown || st.Code != c.code {
				t.Fatalf("state = %+v, want down with %s", st, c.code)
			}
			if !clientClosed(first) || r.client() != nil {
				t.Fatal("the client survived a lost route")
			}
			if n := r.builds.built(); n != 1 {
				t.Fatalf("clients built = %d: nothing may be rebuilt while the route stays lost", n)
			}
			assertStatuses(t, r.m, map[string]Status{"dl": StatusQueued, "seed": StatusCompleted, "failed": StatusFailed})
			if got := mustGet(t, r.m, "failed").Error; got != "disk full" {
				t.Fatalf("error of the failed download = %q", got)
			}
			if !mustGet(t, r.m, "seed").Seeding {
				t.Fatal("the wish to seed was dropped by a lost route")
			}
			if _, err := r.m.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d"); !errors.Is(err, errNetworkDown) {
				t.Fatalf("a request while the route is lost = %v, want errNetworkDown", err)
			}
			if log.count(eventFailed) != 0 {
				t.Fatal("a lost route must not emit download:failed")
			}
			downs := 0
			for _, n := range log.networks() {
				if n.State == NetworkDown {
					downs++
				}
			}
			if downs != 1 {
				t.Fatalf("down events = %d across three checks, want one", downs)
			}

			c.up(r)
			r.reconcile(t)

			if st := r.state(); st.State != NetworkOK {
				t.Fatalf("state after the route came back = %+v", st)
			}
			if r.builds.built() != 2 || r.client() == first {
				t.Fatalf("the route came back without a new client: built %d", r.builds.built())
			}
			waitUntil(t, "the parked downloads to be restored", func() bool {
				return hasEngine(r.m, "dl") && hasEngine(r.m, "seed")
			})
			waitUntil(t, "the queue to start the restored download", func() bool { return r.m.statusOf(t, "dl") == StatusDownloading })
			r.m.wg.Wait()
			assertStatuses(t, r.m, map[string]Status{"dl": StatusDownloading, "seed": StatusCompleted, "failed": StatusFailed})
			if hasEngine(r.m, "failed") {
				t.Fatal("a failed download was restarted behind the user's back")
			}
			if got := mustGet(t, r.m, "failed").Error; got != "disk full" {
				t.Fatalf("error of the failed download = %q after the route came back", got)
			}
			hashes := map[string]bool{}
			for _, tor := range r.client().cl.Torrents() {
				hashes[tor.InfoHash().HexString()] = true
			}
			if !hashes[dl.InfoHash] || !hashes[seed.InfoHash] || len(hashes) != 2 {
				t.Fatalf("torrents on the new client = %v", hashes)
			}
		})
	}
}

func TestUnknownNetworkModeResolvesToNoRoute(t *testing.T) {
	r := newNetRig(t, nil)

	plan, err := r.m.resolveNetwork(t.Context(), settings.Settings{NetworkMode: "tor"}, nil)

	if err == nil {
		t.Fatalf("an unknown mode resolved to %+v: it must never fall back to a direct route", plan)
	}
	if plan != (netPlan{}) {
		t.Fatalf("plan = %+v", plan)
	}
	if !errors.Is(err, errNoClient) {
		t.Fatalf("error = %v, want errNoClient", err)
	}
}

func TestClientBuiltWhileTheManagerLeavesIsClosedAndNotInstalled(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, r *netRig)
	}{
		{"the manager is closing", func(t *testing.T, r *netRig) {
			r.m.mu.Lock()
			r.m.closing = true
			r.m.mu.Unlock()
			r.m.bringUp(t.Context(), settings.Defaults(), netPlan{mode: settings.NetworkDirect})
		}},
		{"the context ends during the build", func(t *testing.T, r *netRig) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r.builds.hold, r.builds.release = make(chan struct{}), make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				r.m.bringUp(ctx, settings.Defaults(), netPlan{mode: settings.NetworkDirect})
			}()
			<-r.builds.hold
			cancel()
			close(r.builds.release)
			<-done
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newNetRig(t, nil)
			r.m.addTestItem("a", StatusQueued)

			c.run(t, r)

			if r.client() != nil {
				t.Fatal("a client built for a manager that is leaving was installed")
			}
			r.builds.mu.Lock()
			built := append([]*client(nil), r.builds.clients...)
			r.builds.mu.Unlock()
			if len(built) != 1 || !clientClosed(built[0]) {
				t.Fatalf("clients built = %d, closed = %v: the unused client must be closed", len(built), len(built) == 1 && clientClosed(built[0]))
			}
			assertStatuses(t, r.m, map[string]Status{"a": StatusQueued})
		})
	}
}

func TestRestorePassStopsWhenItsClientIsGone(t *testing.T) {
	r := newNetRig(t, nil)
	r.reconcile(t)
	realDownload(t, r.m, "a", StatusQueued, false)
	r.m.mu.Lock()
	jobs := r.m.jobsLocked(nil)
	cl := r.m.client
	r.m.mu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	r.m.restorePass(ctx, cl, jobs, true)

	if hasEngine(r.m, "a") {
		t.Fatal("a restore pass whose client is gone still attached an engine")
	}
	assertStatuses(t, r.m, map[string]Status{"a": StatusQueued})
}

// Whether a held port really stops the client depends on the Windows build:
// the CI runner let it bind 42815 under both a TCP and a UDP holder. The
// refusal is therefore the error the client returns for a taken port.
func TestNewClientFallsBackWhenTheFixedPortIsTaken(t *testing.T) {
	var attempts []int
	orig := openTorrentClient
	t.Cleanup(func() { openTorrentClient = orig })
	openTorrentClient = func(tc *torrent.ClientConfig) (*torrent.Client, error) {
		attempts = append(attempts, tc.ListenPort)
		if tc.ListenPort == listenPort {
			return nil, &net.OpError{Op: "listen", Net: "udp", Err: syscall.EADDRINUSE}
		}
		tc.NoDHT = true
		tc.DisableTrackers = true
		tc.DisablePEX = true
		tc.NoDefaultPortForwarding = true
		return orig(tc)
	}

	cl, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), netPlan{mode: settings.NetworkDirect})
	if err != nil {
		t.Fatalf("newClient with the fixed port taken: %v (attempts %v)", err, attempts)
	}
	port := cl.cl.LocalPort()
	cl.close()

	if len(attempts) < 2 || attempts[0] != listenPort {
		t.Fatalf("ports tried = %v, want the fixed one first and a retry after it", attempts)
	}
	for _, p := range attempts[1:] {
		if p != 0 {
			t.Fatalf("ports tried = %v, want every retry on a random port", attempts)
		}
	}
	if port == listenPort {
		t.Fatalf("the client listens on the taken port %d", port)
	}
}
