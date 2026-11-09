// Package netguard decides whether an outgoing connection may go to an
// address that came from external data (a feed URL, a tracker, a webseed, a
// metadata URL). Loopback, private, link-local, multicast and unspecified
// addresses are refused so that a hostile document cannot make the launcher
// talk to the user's own machine or network.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// ErrBlocked reports a destination inside a refused address range.
var ErrBlocked = errors.New("destination is on a private or local network")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// BlockedName refuses the names that always mean the local machine before any
// DNS lookup happens.
func BlockedName(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost")
}

// BlockedAddr refuses addresses that are not routable public unicast.
func BlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return true
	}
	return addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		(addr.Is4() && cgnat.Contains(addr))
}

// DialFunc is the shape of net.Dialer.DialContext.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// ResolveFunc turns a host name into its addresses.
type ResolveFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// Guard bundles the pieces of a guarded dial. Zero values mean the system
// resolver and the launcher-wide BlockedAddr policy.
type Guard struct {
	Resolve ResolveFunc
	Blocked func(netip.Addr) bool
}

// Dial resolves address, refuses it when every resolved address is blocked,
// and otherwise dials the allowed addresses through next, passing the chosen
// IP literal so that the dialled address is the checked one and a second
// lookup cannot swap it (DNS rebinding).
func Dial(ctx context.Context, next DialFunc, resolve ResolveFunc, network, address string) (net.Conn, error) {
	return Guard{Resolve: resolve}.Dial(ctx, next, network, address)
}

// Dial is Dial with the guard's resolver and policy.
func (g Guard) Dial(ctx context.Context, next DialFunc, network, address string) (net.Conn, error) {
	resolve, blocked := g.Resolve, g.Blocked
	if blocked == nil {
		blocked = BlockedAddr
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if BlockedName(host) {
		return nil, fmt.Errorf("%w: %s", ErrBlocked, host)
	}
	var addrs []netip.Addr
	if literal, perr := netip.ParseAddr(host); perr == nil {
		addrs = []netip.Addr{literal}
	} else {
		if resolve == nil {
			resolve = systemResolve
		}
		addrs, err = resolve(ctx, host)
		if err != nil {
			return nil, err
		}
	}
	allowed := addrs[:0:0]
	for _, a := range addrs {
		if !blocked(a) {
			allowed = append(allowed, a)
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrBlocked, host)
	}
	var errs []error
	for _, a := range allowed {
		conn, derr := next(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
		if derr == nil {
			return conn, nil
		}
		errs = append(errs, derr)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errs...)
}

func systemResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}
