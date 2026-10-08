package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDownloadRefusesAnUnusableArtifactBeforeTouchingTheNetwork(t *testing.T) {
	data := bytes.Repeat([]byte("v"), 2048)
	tests := []struct {
		name   string
		mutate func(a *Artifact)
		want   error
	}{
		{"unsupported kind", func(a *Artifact) { a.Kind = "dmg" }, ErrUnsupportedKind},
		{"missing platform", func(a *Artifact) { a.OS = "" }, ErrInvalidArtifact},
		{"name leaves the destination", func(a *Artifact) { a.Name = `..\setup.exe` }, ErrInvalidArtifactName},
		{"empty name", func(a *Artifact) { a.Name = "" }, ErrInvalidArtifactName},
		{"zero size", func(a *Artifact) { a.Size = 0 }, ErrInvalidArtifactSize},
		{"size over the limit", func(a *Artifact) { a.Size = MaxArtifactSize + 1 }, ErrInvalidArtifactSize},
		{"uppercase hash", func(a *Artifact) { a.SHA256 = strings.ToUpper(a.SHA256) }, ErrInvalidHash},
		{"plain http to a public host", func(a *Artifact) { a.URL = "http://downloads.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"missing url", func(a *Artifact) { a.URL = "" }, ErrInvalidArtifactURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				if _, err := w.Write(data); err != nil {
					t.Logf("write body: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL
			tt.mutate(&art)

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			destDir := t.TempDir()
			if _, err := c.Download(context.Background(), art, destDir, nil); !errors.Is(err, tt.want) {
				t.Fatalf("Download() error = %v, want %v", err, tt.want)
			}
			if got := hits.Load(); got != 0 {
				t.Fatalf("server saw %d requests for an artifact the manifest rules should have refused", got)
			}
			assertDirEmpty(t, destDir)
		})
	}
}

func TestDownloadStatusClassification(t *testing.T) {
	shortBackoff(t)
	data := bytes.Repeat([]byte("s"), 2048)

	tests := []struct {
		name         string
		status       int
		body         []byte
		want         error
		wantRequests int32
	}{
		{"bad request", http.StatusBadRequest, []byte("no"), ErrArtifactStatus, 1},
		{"unauthorized", http.StatusUnauthorized, []byte("no"), ErrArtifactStatus, 1},
		{"forbidden", http.StatusForbidden, []byte("no"), ErrArtifactStatus, 1},
		{"gone", http.StatusGone, []byte("no"), ErrArtifactStatus, 1},
		{"not modified", http.StatusNotModified, nil, ErrArtifactStatus, 1},
		{"partial content nobody asked for", http.StatusPartialContent, data, ErrArtifactStatus, 1},
		{"range not satisfiable nobody asked for", http.StatusRequestedRangeNotSatisfiable, nil, ErrArtifactStatus, 1},
		{"too many requests", http.StatusTooManyRequests, []byte("slow down"), ErrArtifactStatus, 3},
		{"internal error", http.StatusInternalServerError, []byte("oops"), ErrArtifactStatus, 3},
		{"bad gateway", http.StatusBadGateway, []byte("oops"), ErrArtifactStatus, 3},
		{"service unavailable", http.StatusServiceUnavailable, []byte("oops"), ErrArtifactStatus, 3},
		{"gateway timeout", http.StatusGatewayTimeout, []byte("oops"), ErrArtifactStatus, 3},
		{"no content is not an installer", http.StatusNoContent, nil, ErrSizeMismatch, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(tt.status)
				if _, err := w.Write(tt.body); err != nil {
					t.Logf("write body: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL
			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			destDir := t.TempDir()
			_, err = c.Download(context.Background(), art, destDir, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Download() error = %v, want %v", err, tt.want)
			}
			if got := hits.Load(); got != tt.wantRequests {
				t.Fatalf("server saw %d requests, want %d", got, tt.wantRequests)
			}
			assertDirEmpty(t, destDir)
		})
	}
}

func TestDownloadDoubtfulPartialContentNeverReachesTheInstaller(t *testing.T) {
	shortBackoff(t)
	data := bytes.Repeat([]byte("abcdefgh"), 512)
	offset := 1000

	tests := []struct {
		name         string
		contentRange string
	}{
		{"total disagrees with the manifest", fmt.Sprintf("bytes %d-%d/%d", offset, len(data)-1, len(data)+1)},
		{"start disagrees with the offset", fmt.Sprintf("bytes %d-%d/%d", offset-1, len(data)-1, len(data))},
		{"header missing", ""},
		{"header unparsable", "bytes abc"},
		{"wrong unit", fmt.Sprintf("items %d-%d/%d", offset, len(data)-1, len(data))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var ranges []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				ranges = append(ranges, r.Header.Get("Range"))
				mu.Unlock()
				if r.Header.Get("Range") == "" {
					if _, err := w.Write(data); err != nil {
						t.Logf("write full body: %v", err)
					}
					return
				}
				if tt.contentRange != "" {
					w.Header().Set("Content-Range", tt.contentRange)
				}
				w.WriteHeader(http.StatusPartialContent)
				if _, err := w.Write(data[offset:]); err != nil {
					t.Logf("write range body: %v", err)
				}
			}))
			t.Cleanup(srv.Close)

			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL
			destDir := t.TempDir()
			seedPartial(t, destDir, art.Name, data[:offset])

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			path, err := c.Download(context.Background(), art, destDir, nil)
			if err != nil {
				t.Fatalf("Download() error = %v, want the download to recover", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read artifact: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("artifact differs from the served bytes")
			}

			mu.Lock()
			defer mu.Unlock()
			if len(ranges) != 2 || ranges[0] != fmt.Sprintf("bytes=%d-", offset) || ranges[1] != "" {
				t.Fatalf("Range headers = %q, want a resume attempt followed by a fresh request: a 206 that does not match the partial must not be spliced in", ranges)
			}
			assertOnlyArtifact(t, destDir, art.Name)
		})
	}
}

func TestDownloadFailedResumeKeepsThePartialBytes(t *testing.T) {
	shortBackoff(t)
	data := bytes.Repeat([]byte("k"), 4096)
	offset := 1500

	drop := func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer cannot be hijacked")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Logf("close hijacked conn: %v", err)
		}
	}
	status := func(code int) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
	}

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"server keeps failing", status(http.StatusServiceUnavailable)},
		{"artifact temporarily missing", status(http.StatusNotFound)},
		{"connection dropped before the headers", drop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			t.Cleanup(srv.Close)

			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL
			destDir := t.TempDir()
			seedPartial(t, destDir, art.Name, data[:offset])

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := c.Download(context.Background(), art, destDir, nil); err == nil {
				t.Fatal("Download() error = nil, want the failure reported")
			}

			got, err := os.ReadFile(filepath.Join(destDir, art.Name+".partial"))
			if err != nil {
				t.Fatalf("partial was lost: %v", err)
			}
			if !bytes.Equal(got, data[:offset]) {
				t.Fatalf("partial holds %d bytes, want the %d it started with: a failed attempt must not cost the user the download so far", len(got), offset)
			}
			if _, err := os.Stat(filepath.Join(destDir, art.Name)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("final artifact exists after a failed download: %v", err)
			}
		})
	}
}

func TestDownloadTamperedResumeFailsInsteadOfLooping(t *testing.T) {
	shortBackoff(t)
	data := bytes.Repeat([]byte("t"), 4096)
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 0xff
	offset := 1024

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) > 6 {
			cancel()
			return
		}
		if r.Header.Get("Range") == "" {
			if _, err := w.Write(tampered); err != nil {
				t.Logf("write full body: %v", err)
			}
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(tampered[offset:]); err != nil {
			t.Logf("write range body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL
	destDir := t.TempDir()
	seedPartial(t, destDir, art.Name, data[:offset])

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Download(ctx, art, destDir, nil); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("Download() error = %v, want ErrHashMismatch", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2: one resume, one fresh retry, then give up", got)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadHidesTheArtifactUntilItIsVerified(t *testing.T) {
	data := bytes.Repeat([]byte("h"), 3*downloadBufSize+17)
	srv := serveBytes(t, data)
	t.Cleanup(srv.Close)

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL
	destDir := t.TempDir()
	finalPath := filepath.Join(destDir, art.Name)

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var calls int
	path, err := c.Download(context.Background(), art, destDir, func(int64) {
		calls++
		if _, statErr := os.Stat(finalPath); !errors.Is(statErr, fs.ErrNotExist) {
			t.Errorf("progress call %d: unverified bytes already sit under the installer's real name: %v", calls, statErr)
		}
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if calls == 0 {
		t.Fatal("no progress reported, the check above never ran")
	}
	if path != finalPath {
		t.Fatalf("Download() path = %q, want %q", path, finalPath)
	}
	if _, err := os.Stat(finalPath + ".partial"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial survived a successful download: %v", err)
	}
}

func TestDownloadDestinationThatIsAFile(t *testing.T) {
	data := bytes.Repeat([]byte("f"), 1024)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if _, err := w.Write(data); err != nil {
			t.Logf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	blocked := filepath.Join(t.TempDir(), "1.2.3")
	writeTestFile(t, blocked, []byte("not a directory"))

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Download(context.Background(), art, blocked, nil); err == nil {
		t.Fatal("Download() error = nil, want an error when the destination is not a directory")
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("server saw %d requests: nothing can be stored, so nothing should be fetched", got)
	}
}
