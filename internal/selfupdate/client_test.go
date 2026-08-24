package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
