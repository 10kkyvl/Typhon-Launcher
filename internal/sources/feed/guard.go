package feed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	connectTimeout        = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
	idleConnTimeout       = 60 * time.Second
	maxIdleConns          = 8
)

var (
	ErrBlockedAddress = errors.New("адрес источника запрещён: локальные и приватные сети недоступны")
	ErrNoAddress      = errors.New("не удалось определить адрес источника")
	ErrTooManyHops    = fmt.Errorf("слишком много редиректов (лимит %d)", MaxRedirects)
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

type resolveFunc func(ctx context.Context, host string) ([]netip.Addr, error)

type guard struct {
	resolve resolveFunc
	blocked func(netip.Addr) bool
}

func NewClient() *http.Client {
	return newGuardedClient(guard{})
}

func newGuardedClient(g guard) *http.Client {
	if g.resolve == nil {
		g.resolve = systemResolve
	}
	if g.blocked == nil {
		g.blocked = blockedAddr
	}
	return &http.Client{
		Timeout:       FetchTimeout,
		CheckRedirect: g.checkRedirect,
		Transport: &http.Transport{
			// A proxy would carry the request past the address guard below.
			Proxy:                 nil,
			DialContext:           g.dial,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
		},
	}
}

func systemResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}

func blockedName(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func blockedAddr(addr netip.Addr) bool {
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

func (g guard) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if blockedName(host) {
		return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, host)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	addrs, err := g.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, ErrNoAddress
	}
	return addrs, nil
}

func (g guard) checkHost(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := g.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	blocked := g.blocked
	if blocked == nil {
		blocked = blockedAddr
	}
	for _, addr := range addrs {
		if blocked(addr) {
			return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, host)
		}
	}
	return addrs, nil
}

func (g guard) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= MaxRedirects {
		return ErrTooManyHops
	}
	if _, err := ValidateURL(req.URL.String()); err != nil {
		return err
	}
	if previous := via[len(via)-1].URL; previous.Host != req.URL.Host || previous.Scheme != req.URL.Scheme {
		req.Header.Del("Authorization")
	}
	_, err := g.checkHost(req.Context(), req.URL.Hostname())
	return err
}

func (g guard) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addrs, err := g.checkHost(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}
	var firstErr error
	for _, addr := range addrs {
		if !matchesNetwork(network, addr) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		return nil, ErrNoAddress
	}
	return nil, firstErr
}

func matchesNetwork(network string, addr netip.Addr) bool {
	switch network {
	case "tcp4", "udp4", "ip4":
		return addr.Unmap().Is4()
	case "tcp6", "udp6", "ip6":
		return !addr.Unmap().Is4()
	default:
		return true
	}
}
