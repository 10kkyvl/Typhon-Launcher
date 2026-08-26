package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func testArtifact(data []byte, name string) Artifact {
	return Artifact{
		OS:     "windows",
		Arch:   "amd64",
		Kind:   KindInstaller,
		Name:   name,
		Size:   int64(len(data)),
		SHA256: hashOf(data),
	}
}

func serveBytes(t *testing.T, data []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(data); err != nil {
			t.Logf("write response body: %v", err)
		}
	}))
}

func TestDownloadSuccess(t *testing.T) {
	data := bytes.Repeat([]byte("typhon-update"), 500)
	srv := serveBytes(t, data)
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
	if want := filepath.Join(destDir, "typhon-setup.exe"); path != want {
		t.Fatalf("Download() path = %q, want %q", path, want)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded content mismatch: got %d bytes, want %d bytes", len(got), len(data))
	}

	if len(reported) == 0 {
		t.Fatalf("onProgress was never called")
	}
	if last := reported[len(reported)-1]; last != int64(len(data)) {
		t.Fatalf("final progress report = %d, want %d", last, len(data))
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("read dest dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dest dir has %d entries, want 1 (no leftover temp files)", len(entries))
	}
}

func TestDownloadTruncatedBody(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 4096)
	short := data[:2048]
	srv := serveBytes(t, short)
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	_, err = c.Download(context.Background(), art, destDir, nil)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("Download() error = %v, want ErrSizeMismatch", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadExcessBody(t *testing.T) {
	base := bytes.Repeat([]byte("x"), 4096)
	extra := append(bytes.Repeat([]byte("x"), 4096), []byte("more-than-declared")...)
	srv := serveBytes(t, extra)
	defer srv.Close()

	art := testArtifact(base, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	_, err = c.Download(context.Background(), art, destDir, nil)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("Download() error = %v, want ErrSizeMismatch", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadHashMismatch(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 4096)
	srv := serveBytes(t, data)
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL
	art.SHA256 = hashOf(bytes.Repeat([]byte("z"), 4096))

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	_, err = c.Download(context.Background(), art, destDir, nil)
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("Download() error = %v, want ErrHashMismatch", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadCancelledBeforeStart(t *testing.T) {
	data := bytes.Repeat([]byte("z"), 4096)
	srv := serveBytes(t, data)
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destDir := t.TempDir()
	_, err = c.Download(ctx, art, destDir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v, want context.Canceled", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadCancelledDuringCopy(t *testing.T) {
	full := bytes.Repeat([]byte("a"), 256*1024)
	half := len(full) / 2
	firstChunkSent := make(chan struct{})
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(full[:half]); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(firstChunkSent)
		<-release
		if _, err := w.Write(full[half:]); err != nil {
			return
		}
	}))
	defer srv.Close()
	defer close(release)

	art := testArtifact(full, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-firstChunkSent
		cancel()
	}()

	destDir := t.TempDir()
	_, err = c.Download(ctx, art, destDir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v, want context.Canceled", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadDestDirMissing(t *testing.T) {
	data := bytes.Repeat([]byte("d"), 1024)
	srv := serveBytes(t, data)
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err = c.Download(context.Background(), art, missing, nil)
	if err == nil {
		t.Fatalf("Download() error = nil, want error for a missing destination dir")
	}
}

func TestDownloadEmptyDestDir(t *testing.T) {
	data := bytes.Repeat([]byte("e"), 1024)
	srv := serveBytes(t, data)
	defer srv.Close()

	art := testArtifact(data, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = c.Download(context.Background(), art, "", nil)
	if !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("Download() error = %v, want ErrEmptyConfigDir", err)
	}
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir %s has %d leftover entries, want 0", dir, len(entries))
	}
}

func TestVerifyFile(t *testing.T) {
	data := bytes.Repeat([]byte("v"), 4096)
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	art := testArtifact(data, "artifact.bin")

	t.Run("valid", func(t *testing.T) {
		if err := VerifyFile(context.Background(), path, art); err != nil {
			t.Fatalf("VerifyFile() error = %v, want nil", err)
		}
	})

	t.Run("size mismatch", func(t *testing.T) {
		bad := art
		bad.Size = art.Size + 1
		err := VerifyFile(context.Background(), path, bad)
		if !errors.Is(err, ErrSizeMismatch) {
			t.Fatalf("VerifyFile() error = %v, want ErrSizeMismatch", err)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		bad := art
		bad.SHA256 = hashOf(bytes.Repeat([]byte("w"), 4096))
		err := VerifyFile(context.Background(), path, bad)
		if !errors.Is(err, ErrHashMismatch) {
			t.Fatalf("VerifyFile() error = %v, want ErrHashMismatch", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		err := VerifyFile(context.Background(), filepath.Join(dir, "missing.bin"), art)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("VerifyFile() error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := VerifyFile(ctx, path, art)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("VerifyFile() error = %v, want context.Canceled", err)
		}
	})
}

func TestDownloadStalled(t *testing.T) {
	full := bytes.Repeat([]byte("s"), 256*1024)
	half := len(full) / 2
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(full[:half]); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		if _, err := w.Write(full[half:]); err != nil {
			return
		}
	}))
	defer srv.Close()
	defer close(release)

	prev := stallTimeout
	stallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { stallTimeout = prev })

	art := testArtifact(full, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	_, err = c.Download(context.Background(), art, destDir, nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("Download() error = %v, want ErrStalled", err)
	}
	assertDirEmpty(t, destDir)
}

func TestDownloadOutlivesRequestTimeout(t *testing.T) {
	full := bytes.Repeat([]byte("l"), 256*1024)
	half := len(full) / 2
	pause := 250 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(full[:half]); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		timer := time.NewTimer(pause)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		if _, err := w.Write(full[half:]); err != nil {
			return
		}
	}))
	defer srv.Close()

	prevTimeout := httpTimeout
	httpTimeout = 50 * time.Millisecond
	t.Cleanup(func() { httpTimeout = prevTimeout })
	prevStall := stallTimeout
	stallTimeout = 5 * time.Second
	t.Cleanup(func() { stallTimeout = prevStall })

	art := testArtifact(full, "typhon-setup.exe")
	art.URL = srv.URL

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	destDir := t.TempDir()
	path, err := c.Download(context.Background(), art, destDir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, full) {
		t.Fatalf("artifact content differs from the served body")
	}
}
