package download

import (
	"errors"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/anacrolix/dht/v2"
	"golang.org/x/net/dns/dnsmessage"
)

// fakeDNS answers every A query with one address and every AAAA query with
// nothing, and remembers where each query came from and what it asked.
type fakeDNS struct {
	pc     net.PacketConn
	answer netip.Addr
	// names limits the answers to these names; empty means any name.
	names map[string]bool

	mu      sync.Mutex
	sources []string
	asked   []string
}

func startDNS(t *testing.T, listen string, answer string, names ...string) *fakeDNS {
	t.Helper()
	pc, err := net.ListenPacket("udp4", listen+":0")
	if err != nil {
		t.Fatalf("listen dns: %v", err)
	}
	f := &fakeDNS{pc: pc, answer: netip.MustParseAddr(answer), names: map[string]bool{}}
	for _, n := range names {
		f.names[n+"."] = true
	}
	t.Cleanup(func() { closeQuietly(pc) })
	go f.serve()
	return f
}

func (f *fakeDNS) port() string {
	_, p, err := net.SplitHostPort(f.pc.LocalAddr().String())
	if err != nil {
		return ""
	}
	return p
}

func (f *fakeDNS) serve() {
	buf := make([]byte, 512)
	for {
		n, from, err := f.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		var req dnsmessage.Message
		if err := req.Unpack(buf[:n]); err != nil || len(req.Questions) == 0 {
			continue
		}
		q := req.Questions[0]
		f.mu.Lock()
		f.sources = append(f.sources, from.String())
		f.asked = append(f.asked, strings.TrimSuffix(q.Name.String(), "."))
		f.mu.Unlock()

		resp := dnsmessage.Message{
			Header:    dnsmessage.Header{ID: req.ID, Response: true, Authoritative: true, RecursionAvailable: true},
			Questions: req.Questions,
		}
		if q.Type == dnsmessage.TypeA && (len(f.names) == 0 || f.names[q.Name.String()]) {
			resp.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: f.answer.As4()},
			}}
		} else if len(f.names) > 0 && !f.names[q.Name.String()] {
			resp.RCode = dnsmessage.RCodeNameError
		}
		out, err := resp.Pack()
		if err != nil {
			continue
		}
		logged(f.pc.WriteTo(out, from))
	}
}

func (f *fakeDNS) queriedFrom() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sources...)
}

func (f *fakeDNS) queried() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

func boundResolver(t *testing.T, dns *fakeDNS, server string) (*bindDialer, nameResolver) {
	t.Helper()
	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context(), dnsPort: dns.port()}
	b.resolver = b.newResolver([]netip.Addr{netip.MustParseAddr(server)})
	return b, nameResolver{r: b.resolver, allow4: true}
}

func TestResolverAsksTheAdapterServersFromTheBoundAddress(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9")
	_, names := boundResolver(t, dns, "127.0.0.3")

	got := names.lookup(t.Context(), "tracker.example")
	if len(got) != 1 || got[0] != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("lookup = %v", got)
	}
	sources := dns.queriedFrom()
	if len(sources) == 0 {
		t.Fatal("the adapter's name server was never asked: the system resolver answered instead")
	}
	for _, s := range sources {
		host, _, _ := strings.Cut(s, ":")
		if host != "127.0.0.2" {
			t.Fatalf("a query came from %s, want the bound address", s)
		}
	}
}

func TestResolverWithoutServersResolvesNothing(t *testing.T) {
	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context()}
	b.resolver = b.newResolver(nil)
	names := nameResolver{r: b.resolver, allow4: true}
	if got := names.lookup(t.Context(), "router.bittorrent.com"); got != nil {
		t.Fatalf("a name resolved with no name server to ask: %v", got)
	}
	conn, err := b.DialContext(t.Context(), "tcp", "router.bittorrent.com:80")
	if err == nil {
		closeQuietly(conn)
		t.Fatal("a host name was dialled with no name server to resolve it")
	}
	// An address needs no resolving and still goes out from the adapter.
	if _, err := b.DialContext(t.Context(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
}

func TestResolverSkipsServersOfAFamilyTheAdapterLacks(t *testing.T) {
	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context()}
	conn, err := b.dnsDial([]netip.Addr{netip.MustParseAddr("fd00::1")})(t.Context(), "udp", "")
	if err == nil {
		closeQuietly(conn)
		t.Fatal("a v6 name server was dialled from an adapter with no v6 address")
	}
	if !errors.Is(err, errNoAdapterDNS) {
		t.Fatalf("error = %v, want errNoAdapterDNS", err)
	}
}

func TestBindDialerResolvesHostNamesThroughTheAdapter(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "127.0.0.1", "peer.example")
	b, _ := boundResolver(t, dns, "127.0.0.3")
	peer, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeQuietly(peer) })
	from := make(chan net.Addr, 1)
	go func() {
		c, err := peer.Accept()
		if err != nil {
			return
		}
		from <- c.RemoteAddr()
		closeQuietly(c)
	}()
	_, port := hostPort(t, peer.Addr().String())

	conn, err := b.DialContext(t.Context(), "tcp", net.JoinHostPort("peer.example", itoa(port)))
	if err != nil {
		t.Fatalf("DialContext by name: %v", err)
	}
	closeQuietly(conn)
	if got := tcpAddrOf(t, <-from).IP.String(); got != "127.0.0.2" {
		t.Fatalf("the peer saw %s, want the bound address", got)
	}
	if asked := dns.queried(); len(asked) == 0 || asked[0] != "peer.example" {
		t.Fatalf("name server saw %v", asked)
	}
}

func TestUDPTrackerHostsAreResolvedThroughTheAdapter(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "tracker.example")
	_, names := boundResolver(t, dns, "127.0.0.3")

	in := [][]string{
		{"udp://tracker.example:6969/announce", "udp://unknown.example:80", "udp://198.51.100.4:80", "http://h.example/a"},
		{"udp://tracker.example:1"},
		{"udp://gone.example:2"},
		{"::bad"},
	}
	got := names.resolveUDPTrackers(t.Context(), in)
	want := [][]string{
		{"udp://203.0.113.9:6969/announce", "udp://198.51.100.4:80", "http://h.example/a"},
		{"udp://203.0.113.9:1"},
	}
	if len(got) != len(want) {
		t.Fatalf("tiers = %v, want %v", got, want)
	}
	for i := range want {
		if strings.Join(got[i], " ") != strings.Join(want[i], " ") {
			t.Fatalf("tier %d = %v, want %v", i, got[i], want[i])
		}
	}
	if len(in[0]) != 4 {
		t.Fatal("the input must not be modified")
	}
	for _, s := range dns.queriedFrom() {
		if !strings.HasPrefix(s, "127.0.0.2:") {
			t.Fatalf("a query came from %s", s)
		}
	}

	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context()}
	b.resolver = b.newResolver(nil)
	blind := nameResolver{r: b.resolver, allow4: true}
	got = blind.resolveUDPTrackers(t.Context(), in)
	if len(got) != 1 || strings.Join(got[0], " ") != "udp://198.51.100.4:80 http://h.example/a" {
		t.Fatalf("with no name server only the address and the http tracker stay: %v", got)
	}
	if v6 := withHost(mustURL(t, "udp://x.example"), netip.MustParseAddr("2001:db8::1")); v6 != "udp://[2001:db8::1]" {
		t.Fatalf("withHost = %q", v6)
	}
}

func TestInterfaceClientRewritesUDPTrackersAndKeepsTheOriginals(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.9", "tracker.example")
	_, names := boundResolver(t, dns, "127.0.0.3")
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x" +
		"&tr=udp%3A%2F%2Ftracker.example%3A6969%2Fannounce&tr=udp%3A%2F%2Fgone.example%3A80"
	cl := offlineClient(t)
	cl.filterTrackers = func(tiers [][]string) [][]string { return names.resolveUDPTrackers(t.Context(), tiers) }
	lt, err := cl.addMagnet(uri, cl.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("addMagnet: %v", err)
	}
	t.Cleanup(lt.drop)

	if got := announced(lt); len(got) != 1 || got[0] != "udp://203.0.113.9:6969/announce" {
		t.Fatalf("trackers of the engine = %v", got)
	}
	var stored []string
	for _, tier := range lt.metainfo().AnnounceList {
		stored = append(stored, tier...)
	}
	if strings.Join(stored, " ") != "udp://tracker.example:6969/announce udp://gone.example:80" {
		t.Fatalf("stored trackers = %v, want the originals", stored)
	}
}

func TestDHTBootstrapIsResolvedThroughTheAdapterOnly(t *testing.T) {
	dns := startDNS(t, "127.0.0.3", "203.0.113.50")
	_, names := boundResolver(t, dns, "127.0.0.3")

	// Another test in the package may have started the refresher with a real
	// client already; then only a run of this test on its own can tell.
	before := goroutineStacks("dnsResolverRefresher")
	nodes, err := names.dhtStartingNodes(t.Context())("udp")()
	if err != nil {
		t.Fatalf("starting nodes: %v", err)
	}
	if len(nodes) != len(dht.DefaultGlobalBootstrapHostPorts) {
		t.Fatalf("%d nodes for %d bootstrap hosts", len(nodes), len(dht.DefaultGlobalBootstrapHostPorts))
	}
	wantPorts := map[string]bool{}
	for _, hp := range dht.DefaultGlobalBootstrapHostPorts {
		_, port, _ := strings.Cut(hp, ":")
		wantPorts[port] = true
	}
	for _, n := range nodes {
		if n.IP().String() != "203.0.113.50" || !wantPorts[itoa(n.Port())] {
			t.Fatalf("node %v", n)
		}
	}
	asked := len(dns.queried())
	if asked == 0 {
		t.Fatal("no bootstrap name went through the adapter's server")
	}
	again, err := names.dhtStartingNodes(t.Context())("udp")()
	if err != nil || len(again) != len(nodes) {
		t.Fatalf("second call = %d, %v", len(again), err)
	}

	if after := goroutineStacks("dnsResolverRefresher"); before == "" && after != "" {
		t.Fatalf("the library global DNS refresher was started by the bootstrap lookup:\n%s", after)
	}

	blind := nameResolver{r: (&bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context()}).newResolver(nil), allow4: true}
	if _, err := blind.dhtStartingNodes(t.Context())("udp")(); err == nil {
		t.Fatal("with no name server there is nothing to bootstrap from")
	}
}

func TestInterfaceConfigTakesTheNamesOutOfTheLibrary(t *testing.T) {
	tc := testConfig(t)
	plan := ifacePlan("10.8.0.2", "")
	plan.dns = "10.8.0.1"
	attach, err := applyNetwork(t.Context(), tc, plan)
	if err != nil {
		t.Fatal(err)
	}
	if tc.DhtStartingNodes == nil || tc.MetainfoSourcesMerger == nil || attach.trackers == nil {
		t.Fatal("bootstrap nodes, metainfo sources and trackers must all go through the adapter resolver")
	}
	if got := plan.dnsServers(); len(got) != 1 || got[0] != netip.MustParseAddr("10.8.0.1") {
		t.Fatalf("dnsServers = %v", got)
	}
	if got := (netPlan{}).dnsServers(); len(got) != 0 {
		t.Fatalf("no servers parsed as %v", got)
	}
	if w := plan.warning(); w != "" {
		t.Fatalf("warning = %q with a name server", w)
	}
	plan.dns = ""
	if w := plan.warning(); w != "download.net_interface_no_dns" {
		t.Fatalf("warning = %q without one", w)
	}
	if w := proxyPlan("socks5").warning(); w != "" {
		t.Fatalf("a proxy has no adapter warning: %q", w)
	}
}

func TestAdapterWithoutDNSIsUpWithAWarning(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.net.setDNS()
	r.reconcile(t)
	st := r.state()
	if st.State != NetworkOK || st.Warning != "download.net_interface_no_dns" || st.Code != "" {
		t.Fatalf("state = %+v", st)
	}

	first := r.client()
	r.net.setDNS("10.8.0.1")
	r.reconcile(t)
	st = r.state()
	if st.State != NetworkOK || st.Warning != "" {
		t.Fatalf("state = %+v", st)
	}
	if r.client() == first || !clientClosed(first) {
		t.Fatal("the resolver is part of the client: a new name server needs a new client")
	}
	if got := r.builds.lastPlan().dnsServers(); len(got) != 1 || got[0] != netip.MustParseAddr("10.8.0.1") {
		t.Fatalf("plan dns = %v", got)
	}

	r.net.mu.Lock()
	r.net.dnsErr = errors.New("registry denied")
	r.net.mu.Unlock()
	r.reconcile(t)
	if st := r.state(); st.State != NetworkDown || st.Code != "download.net_interface_host_check_failed" {
		t.Fatalf("a name server list that cannot be read is not an empty one: %+v", st)
	}
}

func TestInterfaceHostChecks(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"weak host send is on", errNetIfaceWeakHost, "download.net_interface_weak_host"},
		{"the check fails", errNetIfaceHostCheck, "download.net_interface_host_check_failed"},
		{"the system cannot be checked", errNetIfaceUnsupport, "download.net_interface_unsupported"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newNetRig(t, viaInterface)
			r.net.set(ifaceInfo{Name: "vpn0", Index: 7, Up: true, Addrs: []netip.Addr{netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("2001:db8::2")}})
			r.net.mu.Lock()
			r.net.hostErr = c.err
			r.net.mu.Unlock()
			r.reconcile(t)
			if st := r.state(); st.State != NetworkDown || st.Code != c.code {
				t.Fatalf("state = %+v, want down with %s", st, c.code)
			}
			if r.builds.built() != 0 || r.client() != nil {
				t.Fatal("no client may be built on an adapter that cannot be held")
			}
			calls := r.net.checked()
			if len(calls) != 1 || calls[0] != (hostCheckCall{"vpn0", true, true}) {
				t.Fatalf("host check calls = %+v, want one for both families in use", calls)
			}
		})
	}
}

func TestHostCheckRunsOnEveryCheckAndTakesAnUpClientDown(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	first := r.client()
	r.reconcile(t)
	if n := len(r.net.checked()); n != 2 {
		t.Fatalf("host check ran %d times over 2 checks: an unchanged plan is not an unchecked one", n)
	}
	if calls := r.net.checked(); calls[0] != (hostCheckCall{"vpn0", true, false}) {
		t.Fatalf("host check = %+v: only the families in use are asked", calls[0])
	}

	r.net.mu.Lock()
	r.net.hostErr = errNetIfaceWeakHost
	r.net.mu.Unlock()
	r.reconcile(t)
	if st := r.state(); st.State != NetworkDown || st.Code != "download.net_interface_weak_host" {
		t.Fatalf("state = %+v", st)
	}
	if !clientClosed(first) || r.client() != nil {
		t.Fatal("VPN software switching WeakHostSend on must take a running client down")
	}
}

func goroutineStacks(marker string) string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, marker) {
			out = append(out, g)
		}
	}
	return strings.Join(out, "\n")
}
