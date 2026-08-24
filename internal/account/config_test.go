package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestBaseURLDefaultIsProduction(t *testing.T) {
	t.Setenv("TYPHON_API_URL", "")

	got := BaseURL()
	if got != "https://api.typhon-launcher.com" {
		t.Fatalf("default base url = %q", got)
	}
	if _, err := ValidateBaseURL(got); err != nil {
		t.Fatalf("default base url must validate: %v", err)
	}
}

func TestBaseURLPrefersEnv(t *testing.T) {
	t.Setenv("TYPHON_API_URL", "http://127.0.0.1:8080/")

	if got := BaseURL(); got != "http://127.0.0.1:8080" {
		t.Fatalf("base url = %q, want the environment value without the trailing slash", got)
	}
}

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"https", "https://api.example.com", false},
		{"https with path", "https://api.example.com/v1/", false},
		{"http localhost", "http://localhost:8080", false},
		{"http loopback ipv4", "http://127.0.0.1", false},
		{"http loopback ipv6", "http://[::1]:8080", false},
		{"http public", "http://api.example.com", true},
		{"ftp", "ftp://api.example.com", true},
		{"empty", "", true},
		{"no scheme", "api.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateBaseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.raw, err)
			}
			if got == "" {
				t.Fatal("expected non-empty base url")
			}
		})
	}
}

func TestNewClientRejectsInsecureBaseURL(t *testing.T) {
	if _, err := NewClient("http://api.example.com", tokenOK("t")); err == nil {
		t.Fatal("expected error for plain http base url")
	}
}

func newRedirectRequest(t *testing.T, from, to string) (*http.Request, []*http.Request) {
	t.Helper()
	fromURL, err := url.Parse(from)
	if err != nil {
		t.Fatalf("parse %q: %v", from, err)
	}
	toURL, err := url.Parse(to)
	if err != nil {
		t.Fatalf("parse %q: %v", to, err)
	}
	req := &http.Request{URL: toURL, Header: http.Header{}}
	req.Header.Set("Authorization", "Bearer secret")
	return req, []*http.Request{{URL: fromURL, Header: http.Header{}}}
}

func TestCheckRedirectDropsAuthorization(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantErr  bool
		wantAuth bool
	}{
		{"https to http loopback", "https://api.example.com/me", "http://127.0.0.1:9/me", false, false},
		{"https to other host", "https://api.example.com/me", "https://evil.example.com/me", false, false},
		{"https to public http", "https://api.example.com/me", "http://evil.example.com/me", true, false},
		{"same origin", "https://api.example.com/me", "https://api.example.com/v2/me", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, via := newRedirectRequest(t, tt.from, tt.to)
			err := CheckRedirect(req, via)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected redirect to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := req.Header.Get("Authorization") != ""; got != tt.wantAuth {
				t.Fatalf("authorization present = %v, want %v", got, tt.wantAuth)
			}
		})
	}
}

func TestCheckRedirectLimit(t *testing.T) {
	req, _ := newRedirectRequest(t, "https://api.example.com/me", "https://api.example.com/me")
	via := make([]*http.Request, maxRedirects)
	for i := range via {
		via[i] = &http.Request{URL: req.URL, Header: http.Header{}}
	}
	if err := CheckRedirect(req, via); err == nil {
		t.Fatal("expected redirect limit error")
	}
}

func TestClientRedirectAcrossHostsDropsAuthorization(t *testing.T) {
	got := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/me", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	c := newTestClient(t, origin.URL, tokenOK("secret"))
	if _, err := c.Me(t.Context()); err != nil {
		t.Fatalf("me: %v", err)
	}
	if auth := <-got; auth != "" {
		t.Fatalf("authorization leaked across hosts: %q", auth)
	}
}
