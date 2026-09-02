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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var ErrArtifactStatus = errors.New("selfupdate: artifact endpoint returned an error status")

// errArtifactRead separates a body that never arrived from a disk that refused
// to take it: only the first is worth another attempt.
var errArtifactRead = errors.New("download artifact")

const (
	downloadBufSize     = 32 * 1024
	progressMinInterval = 250 * time.Millisecond
)

var (
	stallTimeout    = 60 * time.Second
	downloadBackoff = []time.Duration{2 * time.Second, 5 * time.Second}
)

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s: status %d", ErrArtifactStatus.Error(), e.code)
}

func (e *statusError) Is(target error) bool {
	return target == ErrArtifactStatus
}

func (e *statusError) retryable() bool {
	return e.code == http.StatusTooManyRequests || e.code >= http.StatusInternalServerError
}

// Nothing resumes: the endpoint answers Accept-Ranges: none, so a dropped
// connection costs the whole artifact and the only recovery is another attempt
// from zero.
func (c *Client) Download(ctx context.Context, art Artifact, destDir string, onProgress func(downloaded int64)) (string, error) {
	if err := art.Validate(); err != nil {
		return "", err
	}
	if destDir == "" {
		return "", ErrEmptyConfigDir
	}

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := waitBackoff(ctx, downloadBackoff[attempt-1]); err != nil {
				return "", err
			}
			if onProgress != nil {
				onProgress(0)
			}
		}
		path, err := c.download(ctx, art, destDir, onProgress)
		if err == nil {
			return path, nil
		}
		if attempt == len(downloadBackoff) || !worthRetrying(err) {
			return "", err
		}
		slog.Warn("artifact download failed, retrying",
			"attempt", attempt+1, "attempts", len(downloadBackoff)+1, "error", err)
	}
}

func worthRetrying(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// A host that cannot be reached at all (no VPN, DNS poisoned, port
	// closed) answers the same way every time: retrying only makes the user
	// wait through the dial timeout three times.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return false
	}
	if errors.Is(err, ErrStalled) || errors.Is(err, errArtifactRead) {
		return true
	}
	var status *statusError
	if errors.As(err, &status) {
		return status.retryable()
	}
	return false
}

func waitBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) download(ctx context.Context, art Artifact, destDir string, onProgress func(downloaded int64)) (path string, err error) {
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, art.URL, nil)
	if err != nil {
		return "", fmt.Errorf("build artifact request: %w", err)
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errArtifactRead, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close artifact response body", "error", cerr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &statusError{code: resp.StatusCode}
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
			return written, "", fmt.Errorf("%w: %w", errArtifactRead, rerr)
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
