package download

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

const peerMagnet = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x&x.pe="

// The client is built exactly as the manager builds it, with the real
// newClient, so what is checked here is what runs.
func TestProxyClientHasNoSocketsOfItsOwn(t *testing.T) {
	srv := startSOCKS(t, &socksServer{})
	c, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), socksPlan(t, srv, "", ""))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Cleanup(c.close)

	if n := len(c.cl.Listeners()); n != 0 {
		t.Fatalf("%d listening sockets behind a proxy: each is an address the swarm can reach directly", n)
	}
	if got := c.cl.LocalPort(); got != 0 {
		t.Fatalf("local port = %d", got)
	}
	if !c.httpTrackersOnly {
		t.Fatal("a proxy client must drop the trackers it cannot reach")
	}
}

func TestProxyClientDialsPeersOnlyThroughTheProxy(t *testing.T) {
	const peer = "203.0.113.5:6881"
	cases := []struct {
		name  string
		start func(t *testing.T) (netPlan, func() []string)
	}{
		{"socks5", func(t *testing.T) (netPlan, func() []string) {
			srv := startSOCKS(t, &socksServer{user: "u", pass: "p"})
			return socksPlan(t, srv, "u", "p"), srv.seen
		}},
		{"http connect", func(t *testing.T) (netPlan, func() []string) {
			srv := startConnect(t, &connectServer{})
			return srv.plan(t, "u", "p"), func() []string {
				srv.mu.Lock()
				defer srv.mu.Unlock()
				return append([]string(nil), srv.targets...)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, seen := c.start(t)
			cl, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), plan)
			if err != nil {
				t.Fatalf("newClient: %v", err)
			}
			t.Cleanup(cl.close)
			lt, err := cl.addMagnet(peerMagnet+peer, cl.metaDir, storageOpts{})
			if err != nil {
				t.Fatalf("addMagnet: %v", err)
			}
			t.Cleanup(lt.drop)
			lt.allowDownload()
			lt.allowUpload()

			waitUntil(t, "the client to ask the proxy for the peer", func() bool {
				for _, target := range seen() {
					if target == peer {
						return true
					}
				}
				return false
			})
		})
	}
}

// Binding 127.0.0.2 stands in for the adapter address: 127.0.0.1 is where the
// "peer" listens, so a connection that arrives from anywhere but 127.0.0.2
// did not come from the bound address.
func TestInterfaceClientListensAndDialsOnlyFromTheBoundAddress(t *testing.T) {
	peer, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := peer.Close(); err != nil {
			t.Errorf("close peer: %v", err)
		}
	})
	from := make(chan net.Addr, 4)
	go func() {
		for {
			conn, err := peer.Accept()
			if err != nil {
				return
			}
			from <- conn.RemoteAddr()
			closeQuietly(conn) // only the source address matters
		}
	}()

	plan := netPlan{mode: settings.NetworkInterface, iface: "test", ip4: netip.MustParseAddr("127.0.0.2")}
	completion := nonClosingCompletion{storage.NewMapPieceCompletion()}
	dataDir := t.TempDir()
	var attach netAttach
	cl, tc, err := openTestClient(func() (*torrent.ClientConfig, error) {
		attach.later.stop()
		tc, next, err := networkedConfig(t.Context(), settings.Defaults(), dataDir, 0, completion, plan)
		attach = next
		if err != nil {
			closeDefaultStorage(tc)
			return nil, err
		}
		// Discovery would reach for the internet from a test; what is checked
		// here is which socket carries a peer connection.
		tc.NoDHT = true
		tc.DisableTrackers = true
		tc.DisablePEX = true
		tc.NoDefaultPortForwarding = true
		return tc, nil
	})
	if err != nil {
		attach.later.stop()
		t.Fatalf("open client: %v", err)
	}
	attach.attach(cl)
	c := &client{cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, metaDir: t.TempDir(), completion: completion}
	t.Cleanup(c.close)

	addrs := cl.ListenAddrs()
	if len(addrs) < 2 {
		t.Fatalf("listening on %v, want a tcp and a udp socket", addrs)
	}
	for _, a := range addrs {
		host, _, err := net.SplitHostPort(a.String())
		if err != nil || host != "127.0.0.2" {
			t.Fatalf("a socket listens on %v, want only the bound address", a)
		}
	}

	lt, err := c.addMagnet(peerMagnet+peer.Addr().String(), c.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("addMagnet: %v", err)
	}
	t.Cleanup(lt.drop)
	lt.allowDownload()
	lt.allowUpload()

	select {
	case got := <-from:
		if ip := tcpAddrOf(t, got).IP.String(); ip != "127.0.0.2" {
			t.Fatalf("the peer saw a connection from %s, want the bound address", ip)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the client never dialed the peer")
	}
}
