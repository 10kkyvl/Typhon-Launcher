package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func manifestOfSize(t *testing.T, priv ed25519.PrivateKey, size int) []byte {
	t.Helper()
	m := sampleManifest("")
	pad := 100
	for range 3 {
		m.Notes = strings.Repeat("x", pad)
		out := signManifest(t, priv, m)
		if len(out) == size {
			return out
		}
		pad += size - len(out)
	}
	t.Fatalf("cannot size a signed manifest to %d bytes", size)
	return nil
}

func manifestServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Logf("write manifest body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type bodyAudit struct {
	next   http.RoundTripper
	opened atomic.Int32
	closed atomic.Int32
}

func (a *bodyAudit) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := a.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	a.opened.Add(1)
	resp.Body = &countedBody{ReadCloser: resp.Body, audit: a}
	return resp, nil
}

type countedBody struct {
	io.ReadCloser
	audit *bodyAudit
	once  sync.Once
}

func (b *countedBody) Close() error {
	b.once.Do(func() { b.audit.closed.Add(1) })
	return b.ReadCloser.Close()
}

func auditBodies(c *Client) *bodyAudit {
	a := &bodyAudit{next: c.httpClient.Transport}
	c.httpClient.Transport = a
	c.downloadClient.Transport = a
	return a
}

func TestFetchManifestHappyPath(t *testing.T) {
	priv, pub := testKeyPair(t)
	srv := manifestServer(t, http.StatusOK, signManifest(t, priv, sampleManifest("hello")))
	c, err := newClientWithKey(srv.URL, pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	m, err := c.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest() error = %v", err)
	}
	if m.Version != "1.2.3" || m.Notes != "hello" || len(m.Artifacts) != 1 {
		t.Fatalf("FetchManifest() = %+v", m)
	}
}

func TestFetchManifestBodyLimitBoundary(t *testing.T) {
	priv, pub := testKeyPair(t)
	atLimit := manifestOfSize(t, priv, MaxManifestSize)
	overLimit := append(append([]byte(nil), atLimit...), ' ')

	tests := []struct {
		name string
		body []byte
		want error
	}{
		{"exactly at the limit still verifies", atLimit, nil},
		{"one byte over the limit is refused unread", overLimit, ErrManifestTooLarge},
		{"far over the limit", bytes.Repeat([]byte("x"), MaxManifestSize*4), ErrManifestTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := manifestServer(t, http.StatusOK, tt.body)
			c, err := newClientWithKey(srv.URL, pub)
			if err != nil {
				t.Fatalf("newClientWithKey: %v", err)
			}
			m, err := c.FetchManifest(context.Background())
			if tt.want == nil {
				if err != nil {
					t.Fatalf("FetchManifest() error = %v, want nil", err)
				}
				if m.Version != "1.2.3" {
					t.Fatalf("FetchManifest() version = %q", m.Version)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("FetchManifest() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestFetchManifestStatusDecidesBeforeBody(t *testing.T) {
	priv, pub := testKeyPair(t)
	valid := signManifest(t, priv, sampleManifest(""))

	tests := []struct {
		name   string
		status int
		body   []byte
		want   error
	}{
		{"server error carrying a genuine manifest", http.StatusInternalServerError, valid, ErrManifestStatus},
		{"service unavailable carrying a genuine manifest", http.StatusServiceUnavailable, valid, ErrManifestStatus},
		{"not found", http.StatusNotFound, []byte("nope"), ErrManifestStatus},
		{"forbidden", http.StatusForbidden, []byte("nope"), ErrManifestStatus},
		{"too many requests", http.StatusTooManyRequests, nil, ErrManifestStatus},
		{"redirect without a location", http.StatusMovedPermanently, valid, ErrManifestStatus},
		{"not modified", http.StatusNotModified, nil, ErrManifestStatus},
		{"upgrade required", http.StatusUpgradeRequired, valid, ErrManifestOutdated},
		{"ok with an empty body", http.StatusOK, nil, ErrInvalidManifest},
		{"no content", http.StatusNoContent, nil, ErrInvalidManifest},
		{"ok with an html error page", http.StatusOK, []byte("<html>blocked by your provider</html>"), ErrInvalidManifest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := manifestServer(t, tt.status, tt.body)
			c, err := newClientWithKey(srv.URL, pub)
			if err != nil {
				t.Fatalf("newClientWithKey: %v", err)
			}
			m, err := c.FetchManifest(context.Background())
			if !errors.Is(err, tt.want) {
				t.Fatalf("FetchManifest() error = %v, want %v", err, tt.want)
			}
			if m.Version != "" {
				t.Fatalf("FetchManifest() = %+v alongside an error", m)
			}
		})
	}
}

func TestFetchManifestWithTheReleaseKeyRefusesForeignSignatures(t *testing.T) {
	t.Setenv("TYPHON_DEVMOCK_RELEASE_PUBKEY", "")
	priv, _ := testKeyPair(t)
	srv := manifestServer(t, http.StatusOK, signManifest(t, priv, sampleManifest("")))

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.FetchManifest(context.Background()); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("FetchManifest() error = %v, want ErrBadSignature: a manifest signed with any key but the embedded one must be refused", err)
	}
}

func TestFetchManifestAndDownloadCarryNoCredentials(t *testing.T) {
	priv, pub := testKeyPair(t)
	data := bytes.Repeat([]byte("c"), 2048)
	var leaked atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
			if v := r.Header.Get(h); v != "" {
				leaked.Store(h + ": " + v)
			}
		}
		if r.URL.Path == ManifestPath {
			if _, err := w.Write(signManifest(t, priv, sampleManifest(""))); err != nil {
				t.Logf("write manifest: %v", err)
			}
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Logf("write artifact: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := newClientWithKey(srv.URL, pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	if _, err := c.FetchManifest(context.Background()); err != nil {
		t.Fatalf("FetchManifest() error = %v", err)
	}
	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL + "/typhon-setup.exe"
	if _, err := c.Download(context.Background(), art, t.TempDir(), nil); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if v := leaked.Load(); v != nil {
		t.Fatalf("update traffic carried %v: it is anonymous and must stay so", v)
	}

	probe, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+ManifestPath, nil)
	if err != nil {
		t.Fatalf("build probe: %v", err)
	}
	probe.Header.Set("Authorization", "Bearer probe")
	resp, err := c.httpClient.Do(probe)
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close probe body: %v", err)
	}
	if leaked.Load() == nil {
		t.Fatal("the handler did not notice a deliberate Authorization header, so the check above proves nothing")
	}
}

func TestFetchManifestTimesOutOnAServerThatStopsTalking(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"keyId"`)); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	prev := httpTimeout
	httpTimeout = 100 * time.Millisecond
	t.Cleanup(func() { httpTimeout = prev })

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	_, err = c.FetchManifest(ctx)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("FetchManifest() error = %v, want a timeout: a blocked network must fail fast instead of hanging the check", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("FetchManifest() only returned once the caller's own deadline fired: the client has no deadline of its own for a body that stalls")
	}
}

func TestFetchManifestRedirects(t *testing.T) {
	priv, pub := testKeyPair(t)
	valid := signManifest(t, priv, sampleManifest(""))

	t.Run("same host redirect is followed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == ManifestPath {
				http.Redirect(w, r, "/moved/manifest", http.StatusFound)
				return
			}
			if _, err := w.Write(valid); err != nil {
				t.Logf("write manifest: %v", err)
			}
		}))
		t.Cleanup(srv.Close)
		c, err := newClientWithKey(srv.URL, pub)
		if err != nil {
			t.Fatalf("newClientWithKey: %v", err)
		}
		if _, err := c.FetchManifest(context.Background()); err != nil {
			t.Fatalf("FetchManifest() error = %v", err)
		}
	})

	t.Run("redirect loop stops", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			http.Redirect(w, r, ManifestPath, http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		c, err := newClientWithKey(srv.URL, pub)
		if err != nil {
			t.Fatalf("newClientWithKey: %v", err)
		}
		if _, err := c.FetchManifest(context.Background()); err == nil {
			t.Fatal("FetchManifest() error = nil, want a redirect loop to be cut off")
		}
		if got := hits.Load(); got > 10 {
			t.Fatalf("server saw %d requests, the redirect limit is not applied", got)
		}
	})

	for _, target := range []string{"ftp://127.0.0.1/manifest", "file:///C:/manifest", "http://updates.example.com/launcher/manifest"} {
		t.Run("refuses "+target, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				http.Redirect(w, r, target, http.StatusFound)
			}))
			t.Cleanup(srv.Close)
			c, err := newClientWithKey(srv.URL, pub)
			if err != nil {
				t.Fatalf("newClientWithKey: %v", err)
			}
			if _, err := c.FetchManifest(context.Background()); err == nil {
				t.Fatalf("FetchManifest() error = nil, want the redirect to %s refused", target)
			}
			if got := hits.Load(); got != 1 {
				t.Fatalf("server saw %d requests, want 1", got)
			}
		})
	}
}

func TestDownloadFollowsSameHostRedirect(t *testing.T) {
	data := bytes.Repeat([]byte("r"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/launcher/download/setup.exe" {
			http.Redirect(w, r, "/cdn/setup.exe", http.StatusFound)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Logf("write artifact: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL + "/launcher/download/setup.exe"
	destDir := t.TempDir()
	if _, err := c.Download(context.Background(), art, destDir, nil); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	assertOnlyArtifact(t, destDir, "typhon-setup.exe")
}

func TestDownloadRefusesUnsafeRedirects(t *testing.T) {
	shortBackoff(t)
	data := bytes.Repeat([]byte("u"), 4096)

	for _, target := range []string{"http://downloads.example.com/setup.exe", "ftp://127.0.0.1/setup.exe", "file:///C:/setup.exe"} {
		t.Run(target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			t.Cleanup(srv.Close)

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL
			destDir := t.TempDir()
			if _, err := c.Download(context.Background(), art, destDir, nil); err == nil {
				t.Fatalf("Download() error = nil, want the redirect to %s refused", target)
			}
			assertDirEmpty(t, destDir)
		})
	}
}

func TestResponseBodiesAreClosedOnEveryBranch(t *testing.T) {
	priv, pub := testKeyPair(t)
	valid := signManifest(t, priv, sampleManifest(""))
	data := bytes.Repeat([]byte("b"), 4096)
	shortBackoff(t)

	t.Run("the audit notices a body nobody closed", func(t *testing.T) {
		srv := manifestServer(t, http.StatusOK, valid)
		c, err := NewClient(srv.URL)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		audit := auditBodies(c)
		resp, err := c.httpClient.Get(srv.URL)
		if err != nil {
			t.Fatalf("probe request: %v", err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatalf("drain probe body: %v", err)
		}
		if audit.opened.Load() != 1 || audit.closed.Load() != 0 {
			t.Fatalf("audit opened=%d closed=%d, want 1/0 for a body left open", audit.opened.Load(), audit.closed.Load())
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close probe body: %v", err)
		}
	})

	t.Run("manifest", func(t *testing.T) {
		for _, status := range []int{http.StatusOK, http.StatusInternalServerError, http.StatusUpgradeRequired, http.StatusNotFound} {
			srv := manifestServer(t, status, valid)
			c, err := newClientWithKey(srv.URL, pub)
			if err != nil {
				t.Fatalf("newClientWithKey: %v", err)
			}
			audit := auditBodies(c)
			if _, err := c.FetchManifest(context.Background()); err != nil && status == http.StatusOK {
				t.Fatalf("FetchManifest() error = %v", err)
			}
			if audit.opened.Load() != 1 || audit.closed.Load() != 1 {
				t.Fatalf("status %d: bodies opened=%d closed=%d, want 1/1", status, audit.opened.Load(), audit.closed.Load())
			}
		}
	})

	t.Run("artifact", func(t *testing.T) {
		tests := []struct {
			name    string
			status  int
			body    []byte
			wantErr bool
		}{
			{"success", http.StatusOK, data, false},
			{"hash mismatch", http.StatusOK, bytes.Repeat([]byte("z"), len(data)), true},
			{"not found", http.StatusNotFound, []byte("missing"), true},
			{"retried server error", http.StatusServiceUnavailable, []byte("busy"), true},
			{"unsolicited partial content", http.StatusPartialContent, data, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				srv := manifestServer(t, tt.status, tt.body)
				c, err := NewClient(srv.URL)
				if err != nil {
					t.Fatalf("NewClient: %v", err)
				}
				audit := auditBodies(c)
				art := testArtifact(data, "typhon-setup.exe")
				art.URL = srv.URL
				_, err = c.Download(context.Background(), art, t.TempDir(), nil)
				if (err != nil) != tt.wantErr {
					t.Fatalf("Download() error = %v, wantErr %v", err, tt.wantErr)
				}
				if audit.opened.Load() == 0 || audit.opened.Load() != audit.closed.Load() {
					t.Fatalf("bodies opened=%d closed=%d, every response body must be closed", audit.opened.Load(), audit.closed.Load())
				}
			})
		}
	})
}
