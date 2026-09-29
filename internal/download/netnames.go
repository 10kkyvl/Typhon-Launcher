package download

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
	"golang.org/x/sync/errgroup"
)

const (
	nameLookupLimit = 5 * time.Second
	bootstrapTTL    = 5 * time.Minute
)

var errNoAdapterDNS = errors.New("the adapter has no name server to ask")

// dnsDial is what the resolver dials instead of the servers the system would
// pick: the name servers of the bound adapter, from the bound address. A name
// is then either resolved through the adapter or not at all.
func (b *bindDialer) dnsDial(servers []netip.Addr) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		proto := "udp"
		if strings.HasPrefix(network, "tcp") {
			proto = "tcp"
		}
		var errs []error
		for _, server := range servers {
			local, family := b.ip4, "4"
			if server.Is6() {
				local, family = b.ip6, "6"
			}
			if !local.IsValid() {
				continue
			}
			d := net.Dialer{Timeout: nameLookupLimit, Control: b.control}
			if proto == "tcp" {
				d.LocalAddr = &net.TCPAddr{IP: local.AsSlice()}
			} else {
				d.LocalAddr = &net.UDPAddr{IP: local.AsSlice()}
			}
			port := b.dnsPort
			if port == "" {
				port = "53"
			}
			conn, err := d.DialContext(ctx, proto+family, net.JoinHostPort(server.String(), port))
			if err == nil {
				return conn, nil
			}
			errs = append(errs, err)
			if ctx.Err() != nil {
				break
			}
		}
		if len(errs) == 0 {
			return nil, errNoAdapterDNS
		}
		return nil, errors.Join(errs...)
	}
}

// newResolver forces the Go resolver, which is the only one that honours Dial,
// on every system this builds for.
func (b *bindDialer) newResolver(servers []netip.Addr) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: b.dnsDial(servers)}
}

// nameResolver serves the places where the library would resolve a name on its
// own with the system resolver: UDP tracker hosts and the DHT bootstrap nodes.
type nameResolver struct {
	r      *net.Resolver
	allow4 bool
	allow6 bool
}

func (n nameResolver) lookup(ctx context.Context, host string) []netip.Addr {
	ctx, cancel := context.WithTimeout(ctx, nameLookupLimit)
	defer cancel()
	addrs, err := n.r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		a = a.Unmap()
		if (a.Is4() && n.allow4) || (a.Is6() && n.allow6) {
			out = append(out, a)
		}
	}
	return out
}

// first prefers IPv4: it is what the tracker protocol was written for.
func first(addrs []netip.Addr) (netip.Addr, bool) {
	for _, a := range addrs {
		if a.Is4() {
			return a, true
		}
	}
	if len(addrs) > 0 {
		return addrs[0], true
	}
	return netip.Addr{}, false
}

func isUDPScheme(s string) bool { return s == "udp" || s == "udp4" || s == "udp6" }

// resolveUDPTrackers swaps the host name of every UDP tracker for an address
// resolved through the adapter. The UDP tracker client resolves the name on
// its own, with the system resolver, on every announce; the protocol carries
// no host name, so the address is all the tracker ever needed. A tracker whose
// name does not resolve is dropped.
func (n nameResolver) resolveUDPTrackers(ctx context.Context, tiers [][]string) [][]string {
	hosts := map[string]bool{}
	parsed := make([][]*url.URL, len(tiers))
	for i, tier := range tiers {
		for _, raw := range tier {
			u, err := url.Parse(strings.TrimSpace(raw))
			if err != nil {
				parsed[i] = append(parsed[i], nil)
				continue
			}
			parsed[i] = append(parsed[i], u)
			if isUDPScheme(u.Scheme) {
				if _, err := netip.ParseAddr(u.Hostname()); err != nil {
					hosts[u.Hostname()] = true
				}
			}
		}
	}

	resolved := map[string]netip.Addr{}
	var mu sync.Mutex
	var group errgroup.Group
	for host := range hosts {
		group.Go(func() error {
			if a, ok := first(n.lookup(ctx, host)); ok {
				mu.Lock()
				resolved[host] = a
				mu.Unlock()
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil
	}

	var out [][]string
	for i, tier := range tiers {
		var kept []string
		for j, raw := range tier {
			u := parsed[i][j]
			switch {
			case u == nil:
			case !isUDPScheme(u.Scheme):
				kept = append(kept, raw)
			default:
				if _, err := netip.ParseAddr(u.Hostname()); err == nil {
					kept = append(kept, raw)
					continue
				}
				if a, ok := resolved[u.Hostname()]; ok {
					kept = append(kept, withHost(u, a))
				}
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

func withHost(u *url.URL, a netip.Addr) string {
	c := *u
	if port := u.Port(); port != "" {
		c.Host = net.JoinHostPort(a.String(), port)
	} else if a.Is6() {
		c.Host = "[" + a.String() + "]"
	} else {
		c.Host = a.String()
	}
	return c.String()
}

// dhtStartingNodes replaces the library default, which resolves the bootstrap
// hosts with the system resolver through a global cache of its own. Nothing
// here calls into that cache, so its refresher never starts.
func (n nameResolver) dhtStartingNodes(ctx context.Context) func(string) dht.StartingNodesGetter {
	return func(string) dht.StartingNodesGetter { return n.bootstrapGetter(ctx) }
}

func (n nameResolver) bootstrapGetter(ctx context.Context) dht.StartingNodesGetter {
	var mu sync.Mutex
	var cached []dht.Addr
	var fetched time.Time
	return func() ([]dht.Addr, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(cached) > 0 && time.Since(fetched) < bootstrapTTL {
			return cached, nil
		}
		nodes := n.resolveHostPorts(ctx, dht.DefaultGlobalBootstrapHostPorts)
		if len(nodes) == 0 {
			return nil, errors.New("no bootstrap node resolved through the adapter")
		}
		cached, fetched = nodes, time.Now()
		return cached, nil
	}
}

func (n nameResolver) resolveHostPorts(ctx context.Context, hostPorts []string) []dht.Addr {
	type target struct {
		host string
		port int
	}
	var targets []target
	for _, hp := range hostPorts {
		host, portText, err := net.SplitHostPort(hp)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		targets = append(targets, target{host, port})
	}
	results := make([][]dht.Addr, len(targets))
	var group errgroup.Group
	for i, tg := range targets {
		group.Go(func() error {
			for _, a := range n.lookup(ctx, tg.host) {
				results[i] = append(results[i], dht.NewAddr(&net.UDPAddr{IP: a.AsSlice(), Port: tg.port}))
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil
	}
	var out []dht.Addr
	for _, r := range results {
		out = append(out, r...)
	}
	return out
}
