package download

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// netPlan is everything the torrent client is built from where the network is
// concerned. Two equal plans need the same client, so the type is comparable
// and the monitor rebuilds the client exactly when the plan changes.
type netPlan struct {
	mode string

	iface string
	index uint32
	ip4   netip.Addr
	ip6   netip.Addr
	// dns is the adapter's name servers, comma separated; a slice would make
	// the plan incomparable.
	dns string

	ptype string
	phost string
	pport int
	puser string
	ppass string
}

func joinAddrs(addrs []netip.Addr) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ",")
}

func (p netPlan) dnsServers() []netip.Addr {
	var out []netip.Addr
	for _, part := range strings.Split(p.dns, ",") {
		if a, err := netip.ParseAddr(part); err == nil {
			out = append(out, a)
		}
	}
	return out
}

func (p netPlan) warning() string {
	if p.mode == settings.NetworkInterface && p.dns == "" {
		return uierr.Code(errNetIfaceNoDNS)
	}
	return ""
}

func (p netPlan) proxyAddr() string {
	return net.JoinHostPort(p.phost, strconv.Itoa(p.pport))
}

func (p netPlan) address() string {
	switch p.mode {
	case settings.NetworkInterface:
		var parts []string
		if p.ip4.IsValid() {
			parts = append(parts, p.ip4.String())
		}
		if p.ip6.IsValid() {
			parts = append(parts, p.ip6.String())
		}
		return strings.Join(parts, ", ")
	case settings.NetworkProxy:
		return p.proxyAddr()
	}
	return ""
}

func (p netPlan) proxyURL() *url.URL {
	u := &url.URL{Scheme: p.ptype, Host: p.proxyAddr()}
	if p.puser != "" {
		u.User = url.UserPassword(p.puser, p.ppass)
	}
	return u
}

// netAttach is what has to happen to the client after it exists: the library
// only takes custom dialers from a live client.
type netAttach struct {
	dialers     []torrent.Dialer
	udpListener bool
	// trackers rewrites the tracker list of every torrent the client adds.
	trackers func([][]string) [][]string
}

func (a netAttach) attach(cl *torrent.Client) {
	for _, d := range a.dialers {
		cl.AddDialer(d)
	}
	if !a.udpListener {
		return
	}
	for _, l := range cl.Listeners() {
		if d, ok := l.(torrent.Dialer); ok && strings.HasPrefix(d.DialerNetwork(), "udp") {
			cl.AddDialer(d)
		}
	}
}

// applyNetwork routes every channel of the client according to plan. Each
// channel is either pinned to the plan or switched off; none is left on the
// library default, which would be a direct socket.
func applyNetwork(ctx context.Context, tc *torrent.ClientConfig, p netPlan) (netAttach, error) {
	switch p.mode {
	case settings.NetworkDirect:
		return netAttach{}, nil
	case settings.NetworkInterface:
		return applyInterface(ctx, tc, p)
	case settings.NetworkProxy:
		return applyProxy(tc, p)
	}
	return netAttach{}, fmt.Errorf("unknown network mode %q", p.mode)
}

func applyInterface(ctx context.Context, tc *torrent.ClientConfig, p netPlan) (netAttach, error) {
	if !p.ip4.IsValid() && !p.ip6.IsValid() {
		return netAttach{}, errNetIfaceNoAddr
	}
	b := &bindDialer{ip4: p.ip4, ip6: p.ip6, control: unicastControl(p.index), ctx: ctx}
	b.resolver = b.newResolver(p.dnsServers())
	names := nameResolver{r: b.resolver, allow4: p.ip4.IsValid(), allow6: p.ip6.IsValid()}
	resolveTrackers := func(tiers [][]string) [][]string { return names.resolveUDPTrackers(ctx, tiers) }

	tc.ListenHost = func(network string) string {
		if strings.HasSuffix(network, "6") {
			return p.ip6.String()
		}
		return p.ip4.String()
	}
	tc.DisableIPv4 = !p.ip4.IsValid()
	tc.DisableIPv6 = !p.ip6.IsValid()
	// The sockets the library listens on dial from the wildcard address; peers
	// are dialled from the bound address by the dialers attached afterwards.
	tc.DialForPeerConns = false
	tc.HTTPProxy = nil
	tc.HTTPDialContext = b.DialContext
	tc.TrackerDialContext = b.DialContext
	tc.TrackerListenPacket = b.listenPacket
	tc.NoDefaultPortForwarding = true
	tc.DisableWebtorrent = true
	tc.DhtStartingNodes = names.dhtStartingNodes(ctx)
	tc.MetainfoSourcesMerger = func(t *torrent.Torrent, mi *metainfo.MetaInfo) error {
		spec := torrent.TorrentSpecFromMetaInfo(mi)
		spec.Trackers = resolveTrackers(spec.Trackers)
		return t.MergeSpec(spec)
	}

	a := netAttach{udpListener: true, trackers: resolveTrackers}
	for _, d := range b.peerDialers() {
		a.dialers = append(a.dialers, d)
	}
	return a, nil
}

func applyProxy(tc *torrent.ClientConfig, p netPlan) (netAttach, error) {
	if p.phost == "" || p.pport == 0 {
		return netAttach{}, errProxyNotSet
	}
	var dial dialFunc
	switch p.ptype {
	case settings.ProxySOCKS5:
		d, err := socks5Dial(p)
		if err != nil {
			return netAttach{}, err
		}
		dial = d
	case settings.ProxyHTTP:
		dial = httpConnectDial(p)
	default:
		return netAttach{}, fmt.Errorf("unknown proxy type %q", p.ptype)
	}

	only := proxyOnlyDial(p.phost, p.pport)
	tc.HTTPProxy = http.ProxyURL(p.proxyURL())
	tc.HTTPDialContext = only
	tc.TrackerDialContext = only
	tc.TrackerListenPacket = func(string, string) (net.PacketConn, error) {
		// The library panics when this hook fails; udp trackers are dropped
		// before they are added, so this only ever guards a path that slipped
		// through, and it sends nothing.
		return newBlockedPacketConn(), nil
	}
	tc.DialForPeerConns = false
	tc.AcceptPeerConnections = false
	tc.DisableTCP = true
	tc.DisableUTP = true
	tc.NoDHT = true
	tc.NoDefaultPortForwarding = true
	tc.DisableWebtorrent = true
	tc.DisableIPv6 = true
	tc.MetainfoSourcesMerger = func(t *torrent.Torrent, mi *metainfo.MetaInfo) error {
		spec := torrent.TorrentSpecFromMetaInfo(mi)
		spec.Trackers = httpTrackerTiers(spec.Trackers)
		return t.MergeSpec(spec)
	}
	return netAttach{dialers: []torrent.Dialer{netDialer{network: "tcp", dial: dial}}, trackers: httpTrackerTiers}, nil
}

// httpTrackerTiers keeps the trackers a proxy can carry. UDP trackers need
// their own sockets and a DNS lookup made outside the proxy, and websocket
// trackers belong to WebRTC, which is off.
func httpTrackerTiers(tiers [][]string) [][]string {
	var out [][]string
	for _, tier := range tiers {
		var kept []string
		for _, raw := range tier {
			u, err := url.Parse(strings.TrimSpace(raw))
			if err != nil {
				continue
			}
			if u.Scheme == "http" || u.Scheme == "https" {
				kept = append(kept, raw)
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

func cloneTiers(tiers [][]string) [][]string {
	if tiers == nil {
		return nil
	}
	out := make([][]string, len(tiers))
	for i, tier := range tiers {
		out[i] = append([]string(nil), tier...)
	}
	return out
}
