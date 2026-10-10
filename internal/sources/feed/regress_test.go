package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

const regressMagnet = "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestNewClientIsGuarded(t *testing.T) {
	client := NewClient()
	if client.Timeout != FetchTimeout {
		t.Fatalf("timeout = %v, want %v", client.Timeout, FetchTimeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("redirects are followed without re-validation")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("a proxy would carry feed requests past the address guard")
	}
	if transport.DialContext == nil {
		t.Fatal("connections are not dialled through the address guard")
	}
	if transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 {
		t.Fatalf("transport lacks handshake or header timeouts: %+v", transport)
	}
}

func TestFetchRejectsErrorStatusesThatCarryAValidFeed(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if _, err := w.Write([]byte(validFeedJSON)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			res, err := Fetch(t.Context(), loopbackClient(), srv.URL, Conditional{})
			var statusErr *StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("error = %v, want *StatusError", err)
			}
			if statusErr.StatusCode != status {
				t.Fatalf("status = %d, want %d", statusErr.StatusCode, status)
			}
			if len(res.Feed.Entries) != 0 {
				t.Fatalf("an error reply was parsed as a feed: %+v", res.Feed)
			}
			if !strings.Contains(statusErr.Error(), fmt.Sprint(status)) {
				t.Fatalf("message %q does not name the status", statusErr.Error())
			}
		})
	}
}

func TestFetchNeverReachesTheServerWithACancelledContext(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Fetch(ctx, loopbackClient(), srv.URL, Conditional{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests", n)
	}
}

func TestRedirectTargetsAreValidatedAgain(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   error
	}{
		{"ftp scheme", "ftp://feeds.example.com/feed.json", ErrBadScheme},
		{"file scheme", "file:///etc/passwd", ErrBadScheme},
		{"localhost by name", "http://localhost:9/feed.json", ErrBlockedAddress},
		{"subdomain of localhost", "http://feeds.localhost:9/feed.json", ErrBlockedAddress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) > 1 {
					t.Error("the redirect target was requested")
				}
				http.Redirect(w, r, tc.target, http.StatusFound)
			}))
			defer srv.Close()

			_, err := Fetch(t.Context(), loopbackClient(), srv.URL, Conditional{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMatchesNetwork(t *testing.T) {
	v4 := netip.MustParseAddr("203.0.113.10")
	v6 := netip.MustParseAddr("2606:4700:4700::1111")
	mapped := netip.MustParseAddr("::ffff:203.0.113.10")
	cases := []struct {
		network string
		addr    netip.Addr
		want    bool
	}{
		{"tcp4", v4, true}, {"tcp4", v6, false}, {"tcp4", mapped, true},
		{"tcp6", v6, true}, {"tcp6", v4, false}, {"tcp6", mapped, false},
		{"tcp", v4, true}, {"tcp", v6, true},
		{"udp4", v6, false}, {"ip6", v4, false},
	}
	for _, tc := range cases {
		t.Run(tc.network+"/"+tc.addr.String(), func(t *testing.T) {
			if got := matchesNetwork(tc.network, tc.addr); got != tc.want {
				t.Fatalf("matchesNetwork(%q, %v) = %v, want %v", tc.network, tc.addr, got, tc.want)
			}
		})
	}
}

func feedWith(t *testing.T, name, title, uri string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"name":      name,
		"downloads": []map[string]any{{"title": title, "uri": uri}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseTitleBoundariesCountRunes(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
		valid bool
	}{
		{"blanks and breaks collapse", "  Game \n\t  A B  ", "Game A B", true},
		{"one rune is too short", "A", "", false},
		{"one rune between blanks is too short", "  A  ", "", false},
		{"blank only", " \n\t ", "", false},
		{"two runes is enough", "Aa", "Aa", true},
		{"longest title counts runes not bytes", strings.Repeat("я", MaxTitleLen), strings.Repeat("я", MaxTitleLen), true},
		{"one rune over the limit", strings.Repeat("я", MaxTitleLen+1), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(feedWith(t, "Feed", tc.title, regressMagnet))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !tc.valid {
				if len(f.Entries) != 0 || f.Invalid != 1 {
					t.Fatalf("entries = %d, invalid = %d, want the entry rejected", len(f.Entries), f.Invalid)
				}
				return
			}
			if len(f.Entries) != 1 || f.Entries[0].Title != tc.want {
				t.Fatalf("entries = %+v, want title %q", f.Entries, tc.want)
			}
		})
	}
}

func TestParseFeedNameIsTrimmedAndCutOnARuneBoundary(t *testing.T) {
	f, err := Parse(feedWith(t, "  "+strings.Repeat("Ж", 300)+"  ", "Game A", regressMagnet))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !utf8.ValidString(f.Name) || utf8.RuneCountInString(f.Name) != 200 {
		t.Fatalf("name has %d runes (valid utf-8: %v), want 200", utf8.RuneCountInString(f.Name), utf8.ValidString(f.Name))
	}

	blank, err := Parse(feedWith(t, " \t ", "Game A", regressMagnet))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if blank.Name != "" {
		t.Fatalf("name = %q, want it empty", blank.Name)
	}
}

func TestParseURILengthBoundary(t *testing.T) {
	pad := func(total int) string {
		base := regressMagnet + "&dn="
		return base + strings.Repeat("x", total-len(base))
	}
	atLimit, overLimit := pad(MaxURILen), pad(MaxURILen+1)

	f, err := Parse(feedWith(t, "Feed", "Game A", atLimit))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Entries) != 1 || len(f.Entries[0].URIs) != 1 {
		t.Fatalf("a uri of exactly %d characters was dropped: %+v", MaxURILen, f.Entries)
	}

	f, err = Parse(feedWith(t, "Feed", "Game A", overLimit))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Entries) != 0 || f.Invalid != 1 {
		t.Fatalf("a uri one character over the limit survived: entries=%d invalid=%d", len(f.Entries), f.Invalid)
	}
	if len(f.Warnings) == 0 {
		t.Fatal("the dropped uri was not reported")
	}
}
