package account

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientHardensBothHTTPClients(t *testing.T) {
	c := newTestClient(t, "https://api.example.com", tokenOK("t"))
	clients := map[string]*http.Client{"requests": c.httpClient, "uploads": c.uploadHTTP}
	for name, hc := range clients {
		t.Run(name, func(t *testing.T) {
			if hc.Timeout <= 0 {
				t.Fatal("the client has no overall timeout")
			}
			if hc.CheckRedirect == nil {
				t.Fatal("the client follows redirects without re-validating them")
			}
			transport, ok := hc.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport is %T, want *http.Transport", hc.Transport)
			}
			if transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.DialContext == nil {
				t.Fatalf("transport lacks connect/handshake/header timeouts: %+v", transport)
			}
		})
	}
	if c.uploadHTTP.Timeout <= c.httpClient.Timeout {
		t.Fatalf("uploads get %v, no more than plain requests %v", c.uploadHTTP.Timeout, c.httpClient.Timeout)
	}
	if c.httpClient == c.uploadHTTP {
		t.Fatal("uploads and plain requests share one client")
	}
}

func TestCheckURLSchemeOnlyTrustsRealLoopback(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://api.example.com", true},
		{"HTTPS://API.EXAMPLE.COM/v1", true},
		{"http://localhost:8080", true},
		{"http://LOCALHOST", true},
		{"http://127.0.0.1", true},
		{"http://127.255.255.254:9", true},
		{"http://[::1]:8080", true},

		{"http://example.com", false},
		{"http://localhost.example.com", false},
		{"http://127.0.0.1.example.com", false},
		{"http://127.0.0.1@evil.example.com/", false},
		{"http://localhost@evil.example.com/", false},
		{"http://0.0.0.0", false},
		{"http://10.0.0.1", false},
		{"http://192.168.1.1", false},
		{"http://169.254.169.254", false},
		{"ftp://localhost", false},
		{"ws://localhost", false},
		{"file:///etc/passwd", false},
		{"//localhost/path", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = CheckURLScheme(u)
			if tc.want && err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if !tc.want {
				if err == nil {
					t.Fatal("accepted")
				}
				if !errors.Is(err, ErrInsecureBaseURL) {
					t.Fatalf("error = %v, want ErrInsecureBaseURL", err)
				}
			}
		})
	}
}

func TestClientStopsAtTheRedirectLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	_, err := c.Me(t.Context())
	if got := codeOf(t, err); got != CodeNetwork {
		t.Fatalf("code = %q, want %q", got, CodeNetwork)
	}
	if n := int(hits.Load()); n != maxRedirects {
		t.Fatalf("server saw %d requests, want exactly %d", n, maxRedirects)
	}
}

func TestClientRefusesARedirectToPlainHTTPElsewhere(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://api.example.invalid/v1/me", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("secret"))
	_, err := c.Me(t.Context())
	if got := codeOf(t, err); got != CodeNetwork {
		t.Fatalf("code = %q, want %q", got, CodeNetwork)
	}
	if !errors.Is(err, ErrInsecureBaseURL) {
		t.Fatalf("error = %v, want the insecure-scheme refusal rather than a failed connection", err)
	}
}

func TestClientKeepsAuthorizationOnASameOriginRedirect(t *testing.T) {
	var seen atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == APIPrefix+"/me" {
			http.Redirect(w, r, APIPrefix+"/moved", http.StatusTemporaryRedirect)
			return
		}
		seen.Store(r.Header.Get("Authorization"))
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("secret"))
	if _, err := c.Me(t.Context()); err != nil {
		t.Fatalf("me: %v", err)
	}
	if got := seen.Load(); got != "Bearer secret" {
		t.Fatalf("authorization after a same-origin redirect = %v", got)
	}
}

func TestClientReadsAtMostTheResponseLimit(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), 2*maxResponseBodySize)
	cases := []struct {
		name   string
		status int
		body   []byte
		code   string
	}{
		{"success body past the limit", http.StatusOK, append(append([]byte(`{"id":"u1","bio":"`), huge...), []byte(`"}`)...), CodeServer},
		{"error body past the limit", http.StatusInternalServerError, huge, CodeServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if _, err := w.Write(tc.body); err != nil {
					return
				}
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL, tokenOK("t"))
			user, err := c.Me(t.Context())
			if got := codeOf(t, err); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if user.ID != "" || user.Bio != "" {
				t.Fatalf("a truncated body produced a user: %+v", user.ID)
			}
		})
	}
}

func TestClientChecksTheStatusBeforeDecodingTheBody(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, CodeServer},
		{http.StatusUnauthorized, CodeUnauthenticated},
		{http.StatusForbidden, CodeBlocked},
		{http.StatusNotFound, CodeServer},
		{http.StatusConflict, CodeServer},
		{http.StatusUnprocessableEntity, CodeServer},
		{http.StatusUpgradeRequired, CodeOutdated},
		{http.StatusTooManyRequests, CodeRateLimited},
		{http.StatusInternalServerError, CodeServer},
		{http.StatusBadGateway, CodeServer},
		{http.StatusServiceUnavailable, CodeServer},
		{http.StatusGatewayTimeout, CodeServer},
	}
	seen := map[string]int{}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, CurrentUser{ID: "u1", Username: "playerone"})
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL, tokenOK("t"))
			user, err := c.Me(t.Context())
			if err == nil {
				t.Fatalf("a %d reply was accepted as a user: %+v", tc.status, user)
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *Error", err)
			}
			if apiErr.Code != tc.code || apiErr.Status != tc.status {
				t.Fatalf("got code %q status %d, want %q %d", apiErr.Code, apiErr.Status, tc.code, tc.status)
			}
			if user.ID != "" {
				t.Fatalf("an error reply filled the user: %+v", user)
			}
		})
		seen[tc.code]++
	}
	for _, want := range []string{CodeUnauthenticated, CodeBlocked, CodeOutdated, CodeRateLimited, CodeServer} {
		if seen[want] == 0 {
			t.Fatalf("no case maps to %q", want)
		}
	}
}

func TestClientNetworkFailureKeepsItsCause(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
	}))
	defer srv.Close()

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, release := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer release()

	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline passed", expired, context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, srv.URL, tokenOK("t"))
			_, err := c.Me(tc.ctx)
			if got := codeOf(t, err); got != CodeNetwork {
				t.Fatalf("code = %q, want %q", got, CodeNetwork)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want it to unwrap to %v", err, tc.want)
			}
		})
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests from contexts that were already over", n)
	}
}

func TestClientSendsNoAuthorizationForAnEmptyToken(t *testing.T) {
	var header atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header.Store(r.Header.Values("Authorization"))
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK(""))
	if _, err := c.Me(t.Context()); err != nil {
		t.Fatalf("me: %v", err)
	}
	if got, ok := header.Load().([]string); !ok || len(got) != 0 {
		t.Fatalf("authorization = %v, want no header at all", header.Load())
	}
}

func TestClientTokenLookupFailureIsNotASignOut(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	boom := errors.New("credential vault is locked")
	c := newTestClient(t, srv.URL, func() (string, error) { return "", boom })
	_, err := c.Me(t.Context())
	if got := codeOf(t, err); got != CodeServer {
		t.Fatalf("code = %q, want %q: a broken vault is not a missing session", got, CodeServer)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to unwrap to the vault failure", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests without a token", n)
	}
}

func TestClientLogoutSurfacesServerFailures(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, CodeUnauthenticated},
		{http.StatusTooManyRequests, CodeRateLimited},
		{http.StatusInternalServerError, CodeServer},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			seen := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- r.Method + " " + r.Header.Get("Authorization")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL, tokenOK("ignored"))
			err := c.Logout(t.Context(), "given")
			if got := codeOf(t, err); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if got := <-seen; got != "POST Bearer given" {
				t.Fatalf("request was %q", got)
			}
		})
	}
}
