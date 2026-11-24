package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

// recordedWaits stands in for the clock: every pause the handler asks for is
// summed and returns at once, so a 20-second download takes no time here.
type recordedWaits struct {
	mu    sync.Mutex
	total time.Duration
	count int
}

func (r *recordedWaits) wait(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.total += d
	r.count++
	r.mu.Unlock()
	return ctx.Err()
}

func (r *recordedWaits) take() (time.Duration, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total, count := r.total, r.count
	r.total, r.count = 0, 0
	return total, count
}

func pacedServer(t *testing.T, size int, seconds int) (*releaseServer, []byte, *recordedWaits) {
	t.Helper()
	content := make([]byte, size)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("random artifact: %v", err)
	}
	path := filepath.Join(t.TempDir(), "typhon")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	waits := &recordedWaits{}
	rel := release{version: "1.2.3", path: path, art: selfupdate.Artifact{Name: "typhon-devmock", Size: int64(size)}}
	srv := newReleaseServer([]release{rel}, seconds, waits.wait)
	return srv, content, waits
}

func TestParseFlagsPaceDefault(t *testing.T) {
	cfg, err := parseFlags([]string{"-version", "1.2.3"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.seconds != 20 {
		t.Fatalf("seconds = %d, want 20 by default", cfg.seconds)
	}
	if _, err := parseFlags([]string{"-version", "1.2.3", "-seconds", "-1"}, &bytes.Buffer{}); !errors.Is(err, errBadSeconds) {
		t.Fatalf("parseFlags(-seconds -1) error = %v, want errBadSeconds", err)
	}
}

func TestDownloadTakesAboutTheConfiguredSeconds(t *testing.T) {
	srv, content, waits := pacedServer(t, 200<<10, 20)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	resp, err := testHTTPClient().Get(ts.URL + "/launcher/download/1.2.3/typhon-devmock")
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	got := mustReadAll(t, resp)
	closeBody(t, resp)
	if !bytes.Equal(got, content) {
		t.Fatal("paced body differs from the artifact")
	}
	total, count := waits.take()
	if total < 19*time.Second || total > 21*time.Second {
		t.Fatalf("paced for %v, want about 20s", total)
	}
	if count < 20 {
		t.Fatalf("paused %d times, want the bytes spread out so progress moves steadily", count)
	}
}

func TestPacedDownloadResumesByRange(t *testing.T) {
	srv, content, waits := pacedServer(t, 200<<10, 20)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	half := len(content) / 2
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/launcher/download/1.2.3/typhon-devmock", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", half))
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("GET range: %v", err)
	}
	defer closeBody(t, resp)
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if got := mustReadAll(t, resp); !bytes.Equal(got, content[half:]) {
		t.Fatal("ranged body differs from the second half of the artifact")
	}
	if total, _ := waits.take(); total < 9*time.Second || total > 11*time.Second {
		t.Fatalf("half the file paced for %v, want about 10s at the same rate", total)
	}
}

func TestZeroSecondsServesAtFullSpeed(t *testing.T) {
	srv, content, waits := pacedServer(t, 64<<10, 0)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	resp, err := testHTTPClient().Get(ts.URL + "/launcher/download/1.2.3/typhon-devmock")
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	defer closeBody(t, resp)
	if got := mustReadAll(t, resp); !bytes.Equal(got, content) {
		t.Fatal("body differs from the artifact")
	}
	if total, count := waits.take(); count != 0 {
		t.Fatalf("paused %d times for %v with -seconds 0", count, total)
	}
}
