package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
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
		}
		path, resumed, err := c.download(ctx, art, destDir, onProgress)
		if err == nil {
			return path, nil
		}
		// A resumed download's mismatch cannot be blamed on the fresh bytes
		// alone: the corrupt prefix already came from disk, so one more try
		// from scratch is free and does not need to wait out the backoff.
		if resumed && errors.Is(err, ErrHashMismatch) {
			slog.Warn("resumed artifact hash mismatch, restarting fresh", "error", err)
			attempt--
			continue
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

func removePartial(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove partial artifact", "path", path, "error", err)
	}
}

func parseContentRange(hdr string) (start, total int64, ok bool) {
	var end int64
	if n, err := fmt.Sscanf(hdr, "bytes %d-%d/%d", &start, &end, &total); err != nil || n != 3 {
		return 0, 0, false
	}
	return start, total, true
}

func (c *Client) download(ctx context.Context, art Artifact, destDir string, onProgress func(downloaded int64)) (_ string, resumed bool, err error) {
	partialPath := filepath.Join(destDir, art.Name+".partial")

	f, err := os.OpenFile(partialPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return "", false, fmt.Errorf("open partial artifact: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			if cerr := f.Close(); cerr != nil {
				slog.Warn("close partial artifact", "path", partialPath, "error", cerr)
			}
		}
		if err != nil {
			if info, statErr := os.Stat(partialPath); statErr == nil && info.Size() == 0 {
				removePartial(partialPath)
			}
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return "", false, fmt.Errorf("stat partial artifact: %w", err)
	}

	var offset int64
	hasher := sha256.New()

	switch {
	case info.Size() > art.Size:
		if terr := f.Truncate(0); terr != nil {
			return "", false, fmt.Errorf("truncate oversized partial: %w", terr)
		}
	case info.Size() == art.Size:
		written, sum, herr := copyWithHash(ctx, io.Discard, f, art.Size, 0, nil, nil)
		if herr != nil {
			return "", false, fmt.Errorf("read partial artifact: %w", herr)
		}
		if written == art.Size && sum == art.SHA256 {
			if serr := f.Sync(); serr != nil {
				return "", false, fmt.Errorf("sync artifact: %w", serr)
			}
			if cerr := f.Close(); cerr != nil {
				return "", false, fmt.Errorf("close artifact: %w", cerr)
			}
			closed = true
			finalPath := filepath.Join(destDir, art.Name)
			if rerr := os.Rename(partialPath, finalPath); rerr != nil {
				return "", false, fmt.Errorf("rename artifact: %w", rerr)
			}
			if onProgress != nil {
				onProgress(art.Size)
			}
			return finalPath, false, nil
		}
		if terr := f.Truncate(0); terr != nil {
			return "", false, fmt.Errorf("truncate mismatched partial: %w", terr)
		}
		if _, serr := f.Seek(0, io.SeekStart); serr != nil {
			return "", false, fmt.Errorf("seek partial artifact: %w", serr)
		}
	case info.Size() > 0:
		n, herr := io.Copy(hasher, io.LimitReader(f, info.Size()))
		if herr != nil {
			return "", false, fmt.Errorf("read partial artifact: %w", herr)
		}
		offset = n
		if offset < info.Size() {
			if terr := f.Truncate(offset); terr != nil {
				return "", false, fmt.Errorf("truncate short partial: %w", terr)
			}
		}
	}

	if onProgress != nil {
		onProgress(offset)
	}
	resumed = offset > 0

	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, art.URL, nil)
	if err != nil {
		return "", resumed, fmt.Errorf("build artifact request: %w", err)
	}
	if resumed {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return "", resumed, fmt.Errorf("%w: %w", errArtifactRead, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close artifact response body", "error", cerr)
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		if resumed {
			if terr := f.Truncate(0); terr != nil {
				return "", resumed, fmt.Errorf("truncate partial artifact: %w", terr)
			}
			if _, serr := f.Seek(0, io.SeekStart); serr != nil {
				return "", resumed, fmt.Errorf("seek partial artifact: %w", serr)
			}
			offset = 0
			hasher = sha256.New()
			if onProgress != nil {
				onProgress(0)
			}
		}
	case http.StatusPartialContent:
		if !resumed {
			return "", resumed, &statusError{code: resp.StatusCode}
		}
		hdr := resp.Header.Get("Content-Range")
		start, total, ok := parseContentRange(hdr)
		if !ok || start != offset || total != art.Size {
			rangeErr := fmt.Errorf("%w: content-range %q does not match offset %d", errArtifactRead, hdr, offset)
			if terr := f.Truncate(0); terr != nil {
				return "", resumed, errors.Join(rangeErr, fmt.Errorf("truncate partial artifact: %w", terr))
			}
			return "", resumed, rangeErr
		}
	case http.StatusRequestedRangeNotSatisfiable:
		if !resumed {
			return "", resumed, &statusError{code: resp.StatusCode}
		}
		rangeErr := fmt.Errorf("%w: range rejected", errArtifactRead)
		if terr := f.Truncate(0); terr != nil {
			return "", resumed, errors.Join(rangeErr, fmt.Errorf("truncate partial artifact: %w", terr))
		}
		return "", resumed, rangeErr
	default:
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", resumed, &statusError{code: resp.StatusCode}
		}
	}

	guard := newStallGuard(resp.Body, cancel)
	defer guard.stop()

	written, sum, err := copyWithHash(dlCtx, f, guard, art.Size, offset, hasher, onProgress)
	if err != nil {
		if guard.tripped() && ctx.Err() == nil {
			return "", resumed, fmt.Errorf("%w: %s", ErrStalled, stallTimeout)
		}
		return "", resumed, err
	}
	// Windows refuses to delete an open file, so the partial is closed
	// before it is discarded.
	discard := func() {
		if !closed {
			closed = true
			if cerr := f.Close(); cerr != nil {
				slog.Warn("close partial artifact", "path", partialPath, "error", cerr)
			}
		}
		removePartial(partialPath)
	}
	if written != art.Size {
		discard()
		return "", resumed, ErrSizeMismatch
	}
	if sum != art.SHA256 {
		discard()
		return "", resumed, ErrHashMismatch
	}

	if err := f.Sync(); err != nil {
		return "", resumed, fmt.Errorf("sync artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", resumed, fmt.Errorf("close artifact: %w", err)
	}
	closed = true

	finalPath := filepath.Join(destDir, art.Name)
	if err := os.Rename(partialPath, finalPath); err != nil {
		return "", resumed, fmt.Errorf("rename artifact: %w", err)
	}
	return finalPath, resumed, nil
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

	written, sum, err := copyWithHash(ctx, io.Discard, f, art.Size, 0, nil, nil)
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

func copyWithHash(ctx context.Context, dst io.Writer, src io.Reader, limit, offset int64, hasher hash.Hash, onProgress func(int64)) (int64, string, error) {
	if hasher == nil {
		hasher = sha256.New()
	}
	limited := io.LimitReader(src, limit-offset+1)
	buf := make([]byte, downloadBufSize)
	written := offset
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
