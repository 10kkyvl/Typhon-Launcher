package download

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"typhon/internal/netguard"
)

// guardedHTTPDial wraps the dialer used for HTTP trackers and webseeds. Their
// addresses come from torrent metadata and magnet links, that is from the
// feed, so a torrent must not be able to point the launcher at the user's own
// machine or network (invariant I-7). BitTorrent peers are deliberately not
// routed through this guard: a LAN peer is a legitimate peer.
func guardedHTTPDial(next dialFunc, resolve netguard.ResolveFunc) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		g := netguard.Guard{Resolve: resolve, Blocked: destinationBlocked}
		return g.Dial(ctx, netguard.DialFunc(next), network, address)
	}
}

// destinationBlocked is the address policy for trackers and webseeds, over
// TCP and UDP alike. Tests that stand up a tracker on loopback replace it;
// production always uses the launcher-wide policy.
var destinationBlocked = netguard.BlockedAddr

// listenPacketFunc is the shape of torrent.ClientConfig.TrackerListenPacket.
type listenPacketFunc func(network, address string) (net.PacketConn, error)

// guardedListenPacket wraps the socket the engine announces to UDP trackers
// from. The engine resolves tracker names itself and writes to the address it
// got, so the destination is checked per packet, on the socket. A nil listen
// means the library default.
func guardedListenPacket(listen listenPacketFunc) listenPacketFunc {
	if listen == nil {
		listen = net.ListenPacket
	}
	return func(network, address string) (net.PacketConn, error) {
		pc, err := listen(network, address)
		if err != nil {
			return nil, err
		}
		return guardedPacketConn{PacketConn: pc}, nil
	}
}

type guardedPacketConn struct{ net.PacketConn }

func (c guardedPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	ip, ok := packetDestination(addr)
	if !ok || destinationBlocked(ip) {
		return 0, fmt.Errorf("%w: %s", netguard.ErrBlocked, addr)
	}
	return c.PacketConn.WriteTo(p, addr)
}

// packetDestination extracts the IP a datagram is addressed to; a destination
// that is not an IP literal is refused, since the engine never hands a bare
// name to the socket.
func packetDestination(addr net.Addr) (netip.Addr, bool) {
	if u, ok := addr.(*net.UDPAddr); ok {
		return netip.AddrFromSlice(u.IP)
	}
	if addr == nil {
		return netip.Addr{}, false
	}
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return ap.Addr(), true
}

// directHTTPDial is the guarded dialer for the direct network mode, where the
// library would otherwise open plain sockets to whatever the metadata names.
func directHTTPDial() dialFunc {
	d := &net.Dialer{Timeout: dialTimeout}
	return guardedHTTPDial(d.DialContext, nil)
}

// resolverLookup adapts a net.Resolver to the guard's resolve shape.
func resolverLookup(r *net.Resolver) netguard.ResolveFunc {
	if r == nil {
		return nil
	}
	return func(ctx context.Context, host string) ([]netip.Addr, error) {
		return r.LookupNetIP(ctx, "ip", host)
	}
}
