package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return addr
}

func TestBlockedAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.13.14.15", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"10.0.0.5", true},
		{"172.16.4.1", true},
		{"192.168.1.20", true},
		{"169.254.169.254", true},
		{"fe80::1", true},
		{"fd00::1", true},
		{"fc00::abcd", true},
		{"0.0.0.0", true},
		{"::", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
		{"100.64.0.1", true},
		{"8.8.8.8", false},
		{"203.0.113.10", false},
		{"2606:4700:4700::1111", false},
	}
	for _, c := range cases {
		t.Run(c.addr, func(t *testing.T) {
			if got := blockedAddr(mustAddr(t, c.addr)); got != c.want {
				t.Fatalf("blockedAddr(%s) = %v, want %v", c.addr, got, c.want)
			}
		})
	}
}

func TestBlockedName(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"localhost.", true},
		{"api.localhost", true},
		{"", true},
		{"example.com", false},
		{"localhost.example.com", false},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := blockedName(c.host); got != c.want {
				t.Fatalf("blockedName(%q) = %v, want %v", c.host, got, c.want)
			}
		})
	}
}

func staticResolver(records map[string][]string) resolveFunc {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		values, ok := records[host]
		if !ok {
			return nil, fmt.Errorf("no such host: %s", host)
		}
		addrs := make([]netip.Addr, 0, len(values))
		for _, v := range values {
			addr, err := netip.ParseAddr(v)
			if err != nil {
				return nil, err
			}
			addrs = append(addrs, addr)
		}
		return addrs, nil
	}
}

func TestCheckHostRejectsResolvedPrivateAddresses(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		records map[string][]string
		blocked bool
	}{
		{"public", "feed.example", map[string][]string{"feed.example": {"203.0.113.10"}}, false},
		{"private", "feed.example", map[string][]string{"feed.example": {"192.168.1.20"}}, true},
		{"link local", "feed.example", map[string][]string{"feed.example": {"169.254.169.254"}}, true},
		{"ipv6 unique local", "feed.example", map[string][]string{"feed.example": {"fd00::1"}}, true},
		{"mixed public and private", "feed.example", map[string][]string{"feed.example": {"203.0.113.10", "10.1.2.3"}}, true},
		{"literal loopback", "127.0.0.1", nil, true},
		{"literal ipv6 loopback", "::1", nil, true},
		{"name localhost", "localhost", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := guard{resolve: staticResolver(c.records), blocked: blockedAddr}
			_, err := g.checkHost(context.Background(), c.host)
			if c.blocked {
				if !errors.Is(err, ErrBlockedAddress) {
					t.Fatalf("err = %v, want ErrBlockedAddress", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckHostSurfacesResolverFailure(t *testing.T) {
	g := guard{resolve: staticResolver(nil), blocked: blockedAddr}
	_, err := g.checkHost(context.Background(), "feed.example")
	if err == nil {
		t.Fatal("expected a resolver error")
	}
	if errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("resolver failure reported as a policy block: %v", err)
	}
}

func TestCheckHostRejectsEmptyResolverResult(t *testing.T) {
	g := guard{
		resolve: func(context.Context, string) ([]netip.Addr, error) { return nil, nil },
		blocked: blockedAddr,
	}
	if _, err := g.checkHost(context.Background(), "feed.example"); !errors.Is(err, ErrNoAddress) {
		t.Fatalf("err = %v, want ErrNoAddress", err)
	}
}

func TestFetchRejectsLoopbackWithProductionClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the server")
	}))
	defer srv.Close()

	targets := []string{
		srv.URL,
		"http://localhost/feed.json",
		"http://127.0.0.1:9/feed.json",
		"http://[::1]:9/feed.json",
	}
	for _, raw := range targets {
		t.Run(raw, func(t *testing.T) {
			_, err := Fetch(context.Background(), NewClient(), raw, Conditional{})
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("err = %v, want ErrBlockedAddress", err)
			}
		})
	}
}

func TestFetchRejectsUnsupportedScheme(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com/feed.json", "gopher://example.com"} {
		t.Run(raw, func(t *testing.T) {
			_, err := Fetch(context.Background(), NewClient(), raw, Conditional{})
			if !errors.Is(err, ErrBadScheme) {
				t.Fatalf("err = %v, want ErrBadScheme", err)
			}
		})
	}
}

// rewriteResolver points test hostnames at the local test server and keeps a
// separate address that the injected policy treats as private, so redirects can
// be exercised end to end without touching a real network.
func rewriteResolver(t *testing.T, srv *httptest.Server) (resolveFunc, func(netip.Addr) bool) {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse %s: %v", srv.URL, err)
	}
	local := mustAddr(t, u.Hostname())
	internal := mustAddr(t, "169.254.169.254")
	resolve := func(_ context.Context, name string) ([]netip.Addr, error) {
		switch name {
		case "public.test", "other-public.test":
			return []netip.Addr{local}, nil
		case "internal.test":
			return []netip.Addr{internal}, nil
		}
		return nil, fmt.Errorf("no such host: %s", name)
	}
	blocked := func(addr netip.Addr) bool { return addr == internal }
	return resolve, blocked
}

func testURL(t *testing.T, srv *httptest.Server, host, path string) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse %s: %v", srv.URL, err)
	}
	return "http://" + net.JoinHostPort(host, u.Port()) + path
}

func TestRedirectToPrivateAddressIsBlocked(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, testURL(t, srv, "internal.test", "/"), http.StatusFound)
	}))
	defer srv.Close()

	resolve, blocked := rewriteResolver(t, srv)
	client := newGuardedClient(guard{resolve: resolve, blocked: blocked})

	_, err := Fetch(context.Background(), client, testURL(t, srv, "public.test", "/feed.json"), Conditional{})
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
	if strings.Contains(err.Error(), "/feed.json") {
		t.Fatalf("error leaks the request path: %v", err)
	}
}

func TestRedirectToPublicAddressIsFollowed(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed.json" {
			http.Redirect(w, r, testURL(t, srv, "other-public.test", "/final.json"), http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, validFeedJSON); err != nil {
			t.Errorf("write feed: %v", err)
		}
	}))
	defer srv.Close()

	resolve, blocked := rewriteResolver(t, srv)
	client := newGuardedClient(guard{resolve: resolve, blocked: blocked})

	res, err := Fetch(context.Background(), client, testURL(t, srv, "public.test", "/feed.json"), Conditional{})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(res.Feed.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(res.Feed.Entries))
	}
}

func TestRedirectLimitIsEnforced(t *testing.T) {
	var (
		srv  *httptest.Server
		hops int
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, testURL(t, srv, "public.test", fmt.Sprintf("/hop%d", hops)), http.StatusFound)
	}))
	defer srv.Close()

	resolve, blocked := rewriteResolver(t, srv)
	client := newGuardedClient(guard{resolve: resolve, blocked: blocked})

	_, err := Fetch(context.Background(), client, testURL(t, srv, "public.test", "/feed.json"), Conditional{})
	if !errors.Is(err, ErrTooManyHops) {
		t.Fatalf("err = %v, want ErrTooManyHops", err)
	}
	if hops > MaxRedirects+1 {
		t.Fatalf("server saw %d requests, want at most %d", hops, MaxRedirects+1)
	}
}

func TestFetchErrorHidesQueryString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	resolve, blocked := rewriteResolver(t, srv)
	srv.Close()

	client := newGuardedClient(guard{resolve: resolve, blocked: blocked})

	_, err := Fetch(context.Background(), client, testURL(t, srv, "public.test", "/feed.json?token=s3cret"), Conditional{})
	if err == nil {
		t.Fatal("expected a transport error")
	}
	for _, leak := range []string{"s3cret", "token", "/feed.json"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaks %q: %v", leak, err)
		}
	}
}
