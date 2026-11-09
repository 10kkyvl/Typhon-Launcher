package download

import (
	"context"
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
		g := netguard.Guard{Resolve: resolve, Blocked: httpDestinationBlocked}
		return g.Dial(ctx, netguard.DialFunc(next), network, address)
	}
}

// httpDestinationBlocked is the address policy for trackers and webseeds.
// Tests that stand up a tracker on loopback replace it; production always
// uses the launcher-wide policy.
var httpDestinationBlocked = netguard.BlockedAddr

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
