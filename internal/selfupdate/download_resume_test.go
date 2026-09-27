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
	"sync"
	"testing"
	"time"
)

type rangeBehavior struct {
	full          map[int]bool
	unsatisfiable map[int]bool
	badRange      map[int]bool
}

func rangeServer(t *testing.T, data []byte, requests *[]string, b rangeBehavior) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	count := 0

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		n := count
		*requests = append(*requests, r.Header.Get("Range"))
		mu.Unlock()

		if b.unsatisfiable[n] {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if b.full[n] {
			if _, err := w.Write(data); err != nil {
				t.Logf("write full body: %v", err)
			}
			return
		}

		rangeHdr := r.Header.Get("Range")
		if rangeHdr == "" {
			if _, err := w.Write(data); err != nil {
				t.Logf("write full body: %v", err)
			}
			return
		}

		var start int
		if _, err := fmt.Sscanf(rangeHdr, "bytes=%d-", &start); err != nil {
			t.Fatalf("unparsable Range header %q", rangeHdr)
		}

		if b.badRange[n] {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
			if _, err := w.Write(data); err != nil {
				t.Logf("write bad-range body: %v", err)
			}
			return
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:]); err != nil {
			t.Logf("write range body: %v", err)
		}
	}))
}

func seedPartial(t *testing.T, destDir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(destDir, name+".partial"), content, 0o644); err != nil {
		t.Fatalf("seed partial: %v", err)
	}
}

func TestDownloadResumesExistingPartial(t *testing.T) {
	data := bytes.Repeat([]byte("r"), 8192)
	n := 3000
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, data[:n])

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var reported []int64
	path, err := c.Download(context.Background(), art, destDir, func(v int64) {
		reported = append(reported, v)
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(requests))
	}
	if requests[0] != fmt.Sprintf("bytes=%d-", n) {
		t.Fatalf("Range = %q, want bytes=%d-", requests[0], n)
	}
	if len(reported) == 0 || reported[0] != int64(n) {
		t.Fatalf("first progress report = %v, want %d", reported, n)
	}
	if last := reported[len(reported)-1]; last != int64(len(data)) {
		t.Fatalf("final progress = %d, want %d", last, len(data))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
	if _, err := os.Stat(filepath.Join(destDir, art.Name+".partial")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial file still exists after successful download")
	}
}

func TestDownloadRestartsWhenServerIgnoresRange(t *testing.T) {
	data := bytes.Repeat([]byte("i"), 4096)
	n := 1000
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, data[:n])

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{full: map[int]bool{1: true}})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var reported []int64
	path, err := c.Download(context.Background(), art, destDir, func(v int64) {
		reported = append(reported, v)
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(requests))
	}
	if len(reported) < 2 || reported[0] != int64(n) || reported[1] != 0 {
		t.Fatalf("progress = %v, want to start with %d, 0", reported, n)
	}
	if last := reported[len(reported)-1]; last != int64(len(data)) {
		t.Fatalf("final progress = %d, want %d", last, len(data))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadRestartsOnRangeNotSatisfiable(t *testing.T) {
	shortBackoff(t)

	data := bytes.Repeat([]byte("u"), 4096)
	n := 500
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, data[:n])

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{unsatisfiable: map[int]bool{1: true}})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(requests))
	}
	if requests[1] != "" {
		t.Fatalf("second request Range = %q, want none", requests[1])
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadRestartsOnContentRangeMismatch(t *testing.T) {
	shortBackoff(t)

	data := bytes.Repeat([]byte("m"), 4096)
	n := 700
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, data[:n])

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{badRange: map[int]bool{1: true}})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(requests))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadResumedHashMismatchRestartsFresh(t *testing.T) {
	data := bytes.Repeat([]byte("h"), 4096)
	n := 800
	corrupted := append([]byte(nil), data[:n]...)
	corrupted[0] ^= 0xff
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, corrupted)

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	start := time.Now()
	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("Download() took %s, want no backoff wait", elapsed)
	}
	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(requests))
	}
	if requests[0] != fmt.Sprintf("bytes=%d-", n) {
		t.Fatalf("first request Range = %q, want bytes=%d-", requests[0], n)
	}
	if requests[1] != "" {
		t.Fatalf("second request Range = %q, want none (fresh attempt)", requests[1])
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadCompletePartialNeedsNoRequest(t *testing.T) {
	data := bytes.Repeat([]byte("f"), 4096)
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, data)

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 0 {
		t.Fatalf("server saw %d requests, want 0", len(requests))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
	if _, err := os.Stat(filepath.Join(destDir, art.Name+".partial")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial file still exists")
	}
}

func TestDownloadCompletePartialWithBadHashStartsOver(t *testing.T) {
	data := bytes.Repeat([]byte("g"), 4096)
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, bytes.Repeat([]byte("x"), len(data)))

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 1 || requests[0] != "" {
		t.Fatalf("requests = %q, want one request without a Range header", requests)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded file is %d bytes, want %d: the rejected partial must not leave a hole in front of the fresh bytes", len(got), len(data))
	}
	assertOnlyArtifact(t, destDir, art.Name)
}

func TestDownloadOversizedPartialStartsOver(t *testing.T) {
	data := bytes.Repeat([]byte("o"), 4096)
	oversized := append(append([]byte(nil), data...), []byte("extra-bytes-here")...)
	destDir := t.TempDir()
	art := testArtifact(data, "typhon-setup.exe")
	seedPartial(t, destDir, art.Name, oversized)

	var requests []string
	srv := rangeServer(t, data, &requests, rangeBehavior{})
	defer srv.Close()
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	if len(requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(requests))
	}
	if requests[0] != "" {
		t.Fatalf("request Range = %q, want none", requests[0])
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch")
	}
}
