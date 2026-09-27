package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func shortBackoff(t *testing.T) {
	t.Helper()
	prev := downloadBackoff
	downloadBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { downloadBackoff = prev })
}

func TestDownloadRetriesAfterDroppedConnection(t *testing.T) {
	shortBackoff(t)

	data := bytes.Repeat([]byte("typhon-update"), 4096)
	half := len(data) / 2
	var mu sync.Mutex
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Header.Get("Range"))
		n := len(requests)
		mu.Unlock()

		if n == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			// Declaring the full length and sending half of it makes the
			// server close the connection, which is what the client sees when
			// a link drops mid-artifact.
			if _, err := w.Write(data[:half]); err != nil {
				t.Logf("write half body: %v", err)
			}
			return
		}

		rangeHdr := r.Header.Get("Range")
		var start int
		if _, err := fmt.Sscanf(rangeHdr, "bytes=%d-", &start); err != nil {
			t.Fatalf("second request missing Range header: %q", rangeHdr)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:]); err != nil {
			t.Logf("write remainder: %v", err)
		}
	}))
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	var reported []int64
	path, err := c.Download(context.Background(), art, destDir, func(n int64) {
		reported = append(reported, n)
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(requests))
	}
	if requests[1] != fmt.Sprintf("bytes=%d-", half) {
		t.Fatalf("second request Range = %q, want bytes=%d-", requests[1], half)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded %d bytes, want %d", len(got), len(data))
	}
	if last := reported[len(reported)-1]; last != int64(len(data)) {
		t.Fatalf("final progress report = %d, want %d", last, len(data))
	}
	var sawHalf bool
	for _, n := range reported {
		if sawHalf && n < int64(half) {
			t.Fatalf("progress %v dropped below %d after resuming", reported, half)
		}
		if n == int64(half) {
			sawHalf = true
		}
	}
	if !sawHalf {
		t.Fatalf("progress reports %v never reported the resumed offset %d", reported, half)
	}
	assertOnlyArtifact(t, destDir, "typhon-setup.exe")
}

func TestDownloadRetriesOnServerError(t *testing.T) {
	shortBackoff(t)

	data := bytes.Repeat([]byte("s"), 2048)
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Logf("write body: %v", err)
		}
	}))
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := c.Download(context.Background(), art, t.TempDir(), nil); err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}
}

func TestDownloadDoesNotRetry(t *testing.T) {
	shortBackoff(t)

	data := bytes.Repeat([]byte("n"), 2048)
	cases := []struct {
		name    string
		handler func(w http.ResponseWriter)
		want    error
	}{
		{
			name:    "missing artifact",
			handler: func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) },
			want:    ErrArtifactStatus,
		},
		{
			name: "body that does not match the manifest",
			handler: func(w http.ResponseWriter) {
				if _, err := w.Write(bytes.Repeat([]byte("o"), len(data))); err != nil {
					return
				}
			},
			want: ErrHashMismatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				tc.handler(w)
			}))
			defer srv.Close()

			art := testArtifact(data, "typhon-setup.exe")
			art.URL = srv.URL

			c, err := NewClient(srv.URL)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			destDir := t.TempDir()
			_, err = c.Download(context.Background(), art, destDir, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Download() error = %v, want %v", err, tc.want)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("server saw %d requests, want 1: retrying cannot change this answer", got)
			}
			assertDirEmpty(t, destDir)
		})
	}
}

func TestDownloadRetryStopsOnCancel(t *testing.T) {
	prev := downloadBackoff
	downloadBackoff = []time.Duration{time.Hour, time.Hour}
	t.Cleanup(func() { downloadBackoff = prev })

	data := bytes.Repeat([]byte("c"), 2048)
	var requests atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	defer cancel()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	if _, err := c.Download(ctx, art, destDir, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v, want context.Canceled", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server saw %d requests, want 1: a cancelled update must not wait out the backoff", got)
	}
	assertDirEmpty(t, destDir)
}

func assertOnlyArtifact(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dest dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		t.Fatalf("dest dir holds %d entries, want only %q (abandoned attempts left behind)", len(entries), name)
	}
}
