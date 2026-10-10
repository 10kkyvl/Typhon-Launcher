package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A redirect the policy refuses (plain http to a public host, another scheme,
// a loop) gets the same answer on every attempt. Retrying it burns the backoff
// for nothing and, worse, reports a policy decision as a flaky network.
func TestDownloadDoesNotRetryARefusedRedirect(t *testing.T) {
	shortBackoff(t)

	tests := []struct {
		name   string
		target func(origin string) string
	}{
		{"plain http to a public host", func(string) string { return "http://downloads.example.test/setup.exe" }},
		{"another scheme", func(string) string { return "ftp://downloads.example.test/setup.exe" }},
		{"loop", func(origin string) string { return origin + "/again" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int64
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				http.Redirect(w, r, tt.target(srv.URL), http.StatusFound)
			}))
			defer srv.Close()

			data := []byte("payload")
			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			if _, _, err := c.download(context.Background(), art, t.TempDir(), nil); err == nil {
				t.Fatal("download() error = nil, want the redirect refused")
			}
			perAttempt := hits.Swap(0)

			destDir := t.TempDir()
			_, err = c.Download(context.Background(), art, destDir, nil)
			if !errors.Is(err, ErrInvalidArtifactURL) {
				t.Fatalf("Download() error = %v, want ErrInvalidArtifactURL: a refused redirect is not a network error", err)
			}
			if got := hits.Load(); got != perAttempt {
				t.Fatalf("server saw %d requests, want %d (one attempt): a policy refusal must not be retried", got, perAttempt)
			}
			assertDirEmpty(t, destDir)
		})
	}
}

func TestDownloadStillFollowsAnAllowedRedirect(t *testing.T) {
	shortBackoff(t)

	data := []byte("typhon-update")
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if _, err := w.Write(data); err != nil {
			t.Logf("write body: %v", err)
		}
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/setup.exe", http.StatusFound)
	}))
	defer origin.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = origin.URL
	c, err := NewClient(origin.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := c.Download(context.Background(), art, t.TempDir(), nil); err != nil {
		t.Fatalf("Download() error = %v, want the loopback redirect followed", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("target saw %d requests, want 1", got)
	}
}
