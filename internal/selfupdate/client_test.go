package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"typhon/internal/account"
	"typhon/internal/app"
)

func TestNewClient(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"empty", "", true},
		{"https", "https://updates.example.com", false},
		{"loopback http", "http://127.0.0.1:8080", false},
		{"plain http non loopback", "http://updates.example.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(tc.baseURL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewClient(%q) error = nil, want error", tc.baseURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient(%q) error = %v, want nil", tc.baseURL, err)
			}
			if c == nil {
				t.Fatalf("NewClient(%q) client = nil, want non-nil", tc.baseURL)
			}
		})
	}
}

func TestFetchManifestErrorStatus(t *testing.T) {
	oversized := strings.Repeat("x", MaxManifestSize+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(oversized)); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = c.FetchManifest(context.Background())
	if !errors.Is(err, ErrManifestStatus) {
		t.Fatalf("FetchManifest() error = %v, want ErrManifestStatus", err)
	}
	if errors.Is(err, ErrManifestTooLarge) {
		t.Fatalf("FetchManifest() error = %v, must not read the error body against the size limit", err)
	}
}

func TestFetchManifestContentLengthLies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"keyId":"short"}`)); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	m, err := c.FetchManifest(context.Background())
	if err == nil {
		t.Fatalf("FetchManifest() error = nil, want an error for a truncated body, got manifest %+v", m)
	}
}

func TestFetchManifestSendsVersionHeaders(t *testing.T) {
	var gotUserAgent, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotVersion = r.Header.Get("X-Typhon-Version")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := c.FetchManifest(context.Background()); err == nil {
		t.Fatalf("FetchManifest() error = nil, want an error for status 500")
	}

	if gotUserAgent != account.UserAgent {
		t.Fatalf("User-Agent header = %q, want %q", gotUserAgent, account.UserAgent)
	}
	if gotVersion != app.Version {
		t.Fatalf("X-Typhon-Version header = %q, want %q", gotVersion, app.Version)
	}
}

func TestFetchManifestUsesUnversionedManifestPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := c.FetchManifest(context.Background()); err == nil {
		t.Fatalf("FetchManifest() error = nil, want an error for status 500")
	}

	if gotPath != "/launcher/manifest" {
		t.Fatalf("request path = %q, want %q (self-update must stay unversioned)", gotPath, "/launcher/manifest")
	}
}

func TestFetchManifestOutdatedVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		if _, err := w.Write([]byte("upgrade required")); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = c.FetchManifest(context.Background())
	if !errors.Is(err, ErrManifestOutdated) {
		t.Fatalf("FetchManifest() error = %v, want ErrManifestOutdated", err)
	}
	if errors.Is(err, ErrManifestStatus) {
		t.Fatalf("FetchManifest() error = %v, must not also be ErrManifestStatus", err)
	}
}

func TestFetchManifestRedirectToHTTPBreaks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/launcher/manifest", http.StatusFound)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = c.FetchManifest(context.Background())
	if err == nil {
		t.Fatalf("FetchManifest() error = nil, want redirect to plain http non-loopback to be rejected")
	}
}
