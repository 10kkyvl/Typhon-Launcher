package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var ErrArtifactStatus = errors.New("selfupdate: artifact endpoint returned an error status")

const (
	downloadBufSize     = 32 * 1024
	progressMinInterval = 250 * time.Millisecond
)

var stallTimeout = 60 * time.Second

func (c *Client) Download(ctx context.Context, art Artifact, destDir string, onProgress func(downloaded int64)) (path string, err error) {
	if err := art.Validate(); err != nil {
		return "", err
	}
	if destDir == "" {
		return "", ErrEmptyConfigDir
	}

	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, art.URL, nil)
	if err != nil {
		return "", fmt.Errorf("build artifact request: %w", err)
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download artifact: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close artifact response body", "error", cerr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d", ErrArtifactStatus, resp.StatusCode)
	}

	f, err := os.CreateTemp(destDir, ".selfupdate-*")
	if err != nil {
		return "", fmt.Errorf("create temp artifact file: %w", err)
	}
	tmpPath := f.Name()
	closed := false
	defer func() {
		if !closed {
			if cerr := f.Close(); cerr != nil {
				slog.Warn("close temp artifact file", "path", tmpPath, "error", cerr)
			}
		}
		if path == "" {
			if rerr := os.Remove(tmpPath); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				slog.Warn("remove temp artifact file", "path", tmpPath, "error", rerr)
			}
		}
	}()

	guard := newStallGuard(resp.Body, cancel)
	defer guard.stop()

	written, sum, err := copyWithHash(dlCtx, f, guard, art.Size, onProgress)
	if err != nil {
		if guard.tripped() && ctx.Err() == nil {
			return "", fmt.Errorf("%w: %s", ErrStalled, stallTimeout)
		}
		return "", err
	}
	if written != art.Size {
		return "", ErrSizeMismatch
	}
	if sum != art.SHA256 {
		return "", ErrHashMismatch
	}

	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close artifact: %w", err)
	}
	closed = true

	finalPath := filepath.Join(destDir, art.Name)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("rename artifact: %w", err)
	}
	path = finalPath
	return finalPath, nil
}

func VerifyFile(ctx context.Context, path string, art Artifact) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("close artifact file", "path", path, "error", cerr)
		}
	}()

	written, sum, err := copyWithHash(ctx, io.Discard, f, art.Size, nil)
	if err != nil {
		return err
	}
	if written != art.Size {
		return ErrSizeMismatch
	}
	if sum != art.SHA256 {
		return ErrHashMismatch
	}
	return nil
}

func copyWithHash(ctx context.Context, dst io.Writer, src io.Reader, limit int64, onProgress func(int64)) (int64, string, error) {
	hasher := sha256.New()
	limited := io.LimitReader(src, limit+1)
	buf := make([]byte, downloadBufSize)
	var written int64
	lastReport := time.Now()

	for {
		if err := ctx.Err(); err != nil {
			return written, "", err
		}
		n, rerr := limited.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return written, "", fmt.Errorf("write artifact: %w", werr)
			}
			if _, herr := hasher.Write(buf[:n]); herr != nil {
				return written, "", fmt.Errorf("hash artifact: %w", herr)
			}
			written += int64(n)
			if onProgress != nil && time.Since(lastReport) >= progressMinInterval {
				onProgress(written)
				lastReport = time.Now()
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return written, "", fmt.Errorf("download artifact: %w", rerr)
		}
	}

	if onProgress != nil {
		onProgress(written)
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

type stallGuard struct {
	r     io.Reader
	timer *time.Timer
	fired atomic.Bool
}

func newStallGuard(r io.Reader, cancel context.CancelFunc) *stallGuard {
	g := &stallGuard{r: r}
	g.timer = time.AfterFunc(stallTimeout, func() {
		g.fired.Store(true)
		cancel()
	})
	return g
}

func (g *stallGuard) Read(p []byte) (int, error) {
	n, err := g.r.Read(p)
	if n > 0 {
		g.timer.Reset(stallTimeout)
	}
	return n, err
}

func (g *stallGuard) stop() {
	g.timer.Stop()
}

func (g *stallGuard) tripped() bool {
	return g.fired.Load()
}
