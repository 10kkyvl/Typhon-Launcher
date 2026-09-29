package download

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

func testConfig(t *testing.T) *torrent.ClientConfig {
	t.Helper()
	tc := clientConfig(settings.Defaults(), t.TempDir(), 0, nonClosingCompletion{storage.NewMapPieceCompletion()})
	t.Cleanup(func() { closeDefaultStorage(tc) })
	return tc
}

func ifacePlan(ip4, ip6 string) netPlan {
	p := netPlan{mode: settings.NetworkInterface, iface: "vpn0", index: 7}
	if ip4 != "" {
		p.ip4 = netip.MustParseAddr(ip4)
	}
	if ip6 != "" {
		p.ip6 = netip.MustParseAddr(ip6)
	}
	return p
}

func proxyPlan(kind string) netPlan {
	return netPlan{
		mode: settings.NetworkProxy, ptype: kind, phost: "127.0.0.1", pport: 1080,
		puser: "user", ppass: "p@ss:word",
	}
}

func TestApplyNetworkDirectLeavesTheDefaults(t *testing.T) {
	tc := testConfig(t)
	want := testConfig(t)
	attach, err := applyNetwork(t.Context(), tc, netPlan{mode: settings.NetworkDirect})
	if err != nil {
		t.Fatalf("applyNetwork: %v", err)
	}
	if len(attach.dialers) != 0 || attach.udpListener {
		t.Fatalf("direct mode attaches nothing, got %+v", attach)
	}
	if !tc.DialForPeerConns || !tc.AcceptPeerConnections || tc.DisableTCP || tc.DisableUTP || tc.NoDHT ||
		tc.NoDefaultPortForwarding || tc.DisableWebtorrent || tc.DisableIPv6 || tc.DisableIPv4 {
		t.Fatalf("direct mode changed the client defaults: %+v", tc)
	}
	if tc.HTTPProxy != nil || tc.HTTPDialContext != nil || tc.TrackerDialContext != nil || tc.TrackerListenPacket != nil {
		t.Fatal("direct mode must not install network hooks")
	}
	if tc.ListenPort != want.ListenPort {
		t.Fatalf("listen port changed: %d", tc.ListenPort)
	}
}

func TestApplyNetworkInterface(t *testing.T) {
	cases := []struct {
		name         string
		plan         netPlan
		disable4     bool
		disable6     bool
		host4, host6 string
		dialers      []string
	}{
		{"ipv4 only", ifacePlan("10.8.0.2", ""), false, true, "10.8.0.2", "", []string{"tcp4"}},
		{"both families", ifacePlan("10.8.0.2", "fd00::2"), false, false, "10.8.0.2", "fd00::2", []string{"tcp4", "tcp6"}},
		{"ipv6 only", ifacePlan("", "fd00::2"), true, false, "", "fd00::2", []string{"tcp6"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := testConfig(t)
			attach, err := applyNetwork(t.Context(), tc, c.plan)
			if err != nil {
				t.Fatalf("applyNetwork: %v", err)
			}
			if tc.DisableIPv4 != c.disable4 || tc.DisableIPv6 != c.disable6 {
				t.Errorf("DisableIPv4/6 = %v/%v, want %v/%v", tc.DisableIPv4, tc.DisableIPv6, c.disable4, c.disable6)
			}
			if c.host4 != "" {
				for _, n := range []string{"tcp4", "udp4"} {
					if got := tc.ListenHost(n); got != c.host4 {
						t.Errorf("ListenHost(%s) = %q, want %q", n, got, c.host4)
					}
				}
			}
			if c.host6 != "" {
				for _, n := range []string{"tcp6", "udp6"} {
					if got := tc.ListenHost(n); got != c.host6 {
						t.Errorf("ListenHost(%s) = %q, want %q", n, got, c.host6)
					}
				}
			}
			if tc.DialForPeerConns {
				t.Error("the listening sockets must not dial peers: their TCP dialer has no local address")
			}
			if tc.HTTPProxy != nil {
				t.Error("HTTPProxy must be nil, never taken from the environment")
			}
			if tc.HTTPDialContext == nil || tc.TrackerDialContext == nil || tc.TrackerListenPacket == nil {
				t.Error("http, tracker and udp tracker traffic must be bound")
			}
			if !tc.NoDefaultPortForwarding {
				t.Error("UPnP and NAT-PMP would open the port on the physical router")
			}
			if !tc.DisableWebtorrent {
				t.Error("WebRTC ICE ignores the bound address")
			}
			if tc.NoDHT || tc.DisableUTP || tc.DisableTCP {
				t.Error("uTP and DHT ride the bound sockets and stay on")
			}
			if !attach.udpListener {
				t.Error("the bound uTP sockets must be attached as dialers")
			}
			var got []string
			for _, d := range attach.dialers {
				got = append(got, d.DialerNetwork())
			}
			if len(got) != len(c.dialers) {
				t.Fatalf("dialers = %v, want %v", got, c.dialers)
			}
			for i := range got {
				if got[i] != c.dialers[i] {
					t.Fatalf("dialers = %v, want %v", got, c.dialers)
				}
			}
		})
	}
}

func TestApplyNetworkInterfaceWithoutAddress(t *testing.T) {
	_, err := applyNetwork(t.Context(), testConfig(t), ifacePlan("", ""))
	if !errors.Is(err, errNetIfaceNoAddr) {
		t.Fatalf("error = %v, want errNetIfaceNoAddr", err)
	}
}

func TestApplyNetworkProxy(t *testing.T) {
	cases := []struct {
		kind   string
		scheme string
	}{
		{settings.ProxySOCKS5, "socks5"},
		{settings.ProxyHTTP, "http"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			tc := testConfig(t)
			attach, err := applyNetwork(t.Context(), tc, proxyPlan(c.kind))
			if err != nil {
				t.Fatalf("applyNetwork: %v", err)
			}
			if tc.DialForPeerConns || tc.AcceptPeerConnections {
				t.Error("no peer may dial or be accepted outside the proxy")
			}
			if !tc.DisableTCP || !tc.DisableUTP || !tc.NoDHT {
				t.Error("no listening socket, no uTP and no DHT behind a proxy")
			}
			if !tc.NoDefaultPortForwarding || !tc.DisableWebtorrent || !tc.DisableIPv6 {
				t.Error("port forwarding, WebRTC and IPv6 must be off")
			}
			if tc.MetainfoSourcesMerger == nil {
				t.Error("metainfo sources must not bring udp trackers back")
			}
			if attach.udpListener || len(attach.dialers) != 1 || attach.dialers[0].DialerNetwork() != "tcp" {
				t.Fatalf("attach = %+v, want one tcp dialer through the proxy", attach)
			}

			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:6969/announce", nil)
			if err != nil {
				t.Fatal(err)
			}
			u, err := tc.HTTPProxy(req)
			if err != nil || u == nil {
				t.Fatalf("HTTPProxy = %v, %v: even a loopback tracker goes through the proxy", u, err)
			}
			if u.Scheme != c.scheme || u.Host != "127.0.0.1:1080" {
				t.Fatalf("proxy url = %s", u.Redacted())
			}
			if pass, _ := u.User.Password(); u.User.Username() != "user" || pass != "p@ss:word" {
				t.Fatalf("proxy credentials lost: %v", u.User)
			}

			for name, dial := range map[string]func(context.Context, string, string) (net.Conn, error){
				"http":    tc.HTTPDialContext,
				"tracker": tc.TrackerDialContext,
			} {
				conn, err := dial(t.Context(), "tcp", "198.51.100.7:443")
				if err == nil {
					closeQuietly(conn) // the test failed already; only the dial matters
					t.Fatalf("%s transport dialed a host other than the proxy", name)
				}
				if !errors.Is(err, errDirectBlocked) {
					t.Errorf("%s error = %v, want errDirectBlocked", name, err)
				}
			}

			pc, err := tc.TrackerListenPacket("udp4", ":0")
			if err != nil {
				t.Fatalf("TrackerListenPacket: %v", err)
			}
			t.Cleanup(func() { closeQuietly(pc) }) // the socket is a stub with nothing to release
			if _, err := pc.WriteTo([]byte("x"), &net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 6969}); !errors.Is(err, errDirectBlocked) {
				t.Fatalf("a udp tracker packet must never leave: %v", err)
			}
		})
	}
}

func TestApplyNetworkRejectsIncompleteProxy(t *testing.T) {
	cases := []struct {
		name string
		plan netPlan
		want error
	}{
		{"no host", netPlan{mode: settings.NetworkProxy, ptype: "socks5", pport: 1080}, errProxyNotSet},
		{"no port", netPlan{mode: settings.NetworkProxy, ptype: "socks5", phost: "127.0.0.1"}, errProxyNotSet},
	}
	for _, c := range cases {
		if _, err := applyNetwork(t.Context(), testConfig(t), c.plan); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := applyNetwork(t.Context(), testConfig(t), netPlan{mode: settings.NetworkProxy, ptype: "ftp", phost: "h", pport: 1}); err == nil {
		t.Error("unknown proxy type must be an error")
	}
	if _, err := applyNetwork(t.Context(), testConfig(t), netPlan{mode: "vpn"}); err == nil {
		t.Error("unknown mode must be an error, not a direct client")
	}
}

func TestHTTPTrackerTiers(t *testing.T) {
	in := [][]string{
		{"udp://tracker.example:6969/announce", "http://a.example/announce"},
		{"wss://tracker.example/announce"},
		{"https://b.example/announce", "udp://c.example:80"},
		{"::not a url"},
	}
	got := httpTrackerTiers(in)
	want := [][]string{{"http://a.example/announce"}, {"https://b.example/announce"}}
	if len(got) != len(want) {
		t.Fatalf("tiers = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != 1 || got[i][0] != want[i][0] {
			t.Fatalf("tiers = %v, want %v", got, want)
		}
	}
	if len(in[0]) != 2 {
		t.Fatal("the input must not be modified")
	}
	if httpTrackerTiers(nil) != nil {
		t.Fatal("no trackers stay no trackers")
	}
}

// A proxy client drops the trackers it cannot use, but the torrent kept on
// disk must carry all of them: switching the proxy off later has to bring the
// udp trackers back.
func TestProxyClientKeepsTheFullTrackerListInStoredMetainfo(t *testing.T) {
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x" +
		"&tr=udp%3A%2F%2Ftracker.example%3A6969%2Fannounce&tr=http%3A%2F%2Fhttp.example%2Fannounce&tr=wss%3A%2F%2Fws.example"
	cl := offlineClient(t)
	cl.httpTrackersOnly = true
	cl.filterTrackers = httpTrackerTiers
	lt, err := cl.addMagnet(uri, cl.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("addMagnet: %v", err)
	}
	t.Cleanup(lt.drop)

	live := announced(lt)
	if len(live) != 1 || live[0] != "http://http.example/announce" {
		t.Fatalf("trackers on the proxy client = %v, want only the http one", live)
	}
	stored := lt.metainfo().AnnounceList
	var flat []string
	for _, tier := range stored {
		flat = append(flat, tier...)
	}
	if len(flat) != 3 {
		t.Fatalf("stored trackers = %v, want all three", flat)
	}

	direct := offlineClient(t)
	lt2, err := direct.addMagnet(uri, direct.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("addMagnet: %v", err)
	}
	t.Cleanup(lt2.drop)
	if got := announced(lt2); len(got) != 3 {
		t.Fatalf("a direct client must keep every tracker, got %v", got)
	}
}
