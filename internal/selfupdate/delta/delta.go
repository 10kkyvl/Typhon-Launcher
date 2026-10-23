// Package delta builds and applies binary patches between two builds of the
// launcher. A patch is a zstd frame that was compressed with the old build as
// a raw dictionary, the same idea as `zstd --patch-from`: the encoder finds
// the bytes the new build shares with the old one and spends output only on
// what changed.
package delta

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// dictID is written into the frame header and has to be non-zero: a frame
// without a dictionary id is decoded without any dictionary at all.
const dictID = 1

const (
	minWindowBits = 20
	maxWindowBits = 28
	maxWindow     = 1 << maxWindowBits
)

var (
	ErrEmptyBase      = errors.New("delta: the base build is empty")
	ErrEmptyTarget    = errors.New("delta: the target build is empty")
	ErrTooLarge       = errors.New("delta: the builds are too large for a patch")
	ErrOutputTooLarge = errors.New("delta: the patch expands beyond the expected size")
	ErrCorrupt        = errors.New("delta: the patch is damaged or was made against another build")
	ErrBadLimit       = errors.New("delta: the output limit must be positive")
)

// windowBits is the exponent of the smallest power of two that reaches from the
// end of the output back to the start of the base: a match against any byte of
// the base is only legal inside the window.
func windowBits(base, target int) (int, error) {
	if base > maxWindow || target > maxWindow || base+target > maxWindow {
		return 0, fmt.Errorf("%w: %d bytes of base and %d of target", ErrTooLarge, base, target)
	}
	bits := minWindowBits
	for 1<<bits < base+target {
		bits++
	}
	return bits, nil
}

// Encode writes the patch that turns base into target.
func Encode(w io.Writer, base, target []byte) error {
	if len(base) == 0 {
		return ErrEmptyBase
	}
	if len(target) == 0 {
		return ErrEmptyTarget
	}
	bits, err := windowBits(len(base), len(target))
	if err != nil {
		return err
	}
	enc, err := zstd.NewWriter(w,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderDictRaw(dictID, base),
		zstd.WithWindowSize(1<<bits),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return fmt.Errorf("delta: create encoder: %w", err)
	}
	if _, err := enc.Write(target); err != nil {
		return errors.Join(fmt.Errorf("delta: encode: %w", err), enc.Close())
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("delta: finish the patch: %w", err)
	}
	return nil
}

// Apply writes the build that patch describes against base to dst and
// returns how many bytes it wrote. It refuses to write more than limit bytes
// and stops when ctx is done. A patch is not a proof of anything: the caller
// checks the hash of what came out.
func Apply(ctx context.Context, dst io.Writer, base []byte, patch io.Reader, limit int64) (int64, error) {
	if len(base) == 0 {
		return 0, ErrEmptyBase
	}
	if limit <= 0 {
		return 0, ErrBadLimit
	}
	if limit > maxWindow {
		return 0, fmt.Errorf("%w: output limit %d", ErrTooLarge, limit)
	}
	bits, err := windowBits(len(base), int(limit))
	if err != nil {
		return 0, err
	}
	win := uint64(1) << bits
	dec, err := zstd.NewReader(&ctxReader{ctx: ctx, r: patch},
		zstd.WithDecoderDictRaw(dictID, base),
		zstd.WithDecoderMaxWindow(win),
		zstd.WithDecoderMaxMemory(win),
		zstd.WithDecoderConcurrency(1),
	)
	if err != nil {
		return 0, fmt.Errorf("delta: create decoder: %w", err)
	}
	defer dec.Close()

	out := &limitWriter{ctx: ctx, w: dst, left: limit}
	n, err := io.Copy(out, dec)
	switch {
	case err == nil && n == 0:
		return 0, fmt.Errorf("%w: the patch produced nothing", ErrCorrupt)
	case err == nil:
		return n, nil
	case errors.Is(err, errLimit):
		return n, ErrOutputTooLarge
	case ctx.Err() != nil:
		return n, ctx.Err()
	case errors.Is(err, errWrite):
		return n, err
	default:
		return n, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
}

var (
	errLimit = errors.New("delta: output limit")
	errWrite = errors.New("delta: write the output")
)

type limitWriter struct {
	ctx  context.Context
	w    io.Writer
	left int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > l.left {
		return 0, errLimit
	}
	n, err := l.w.Write(p)
	l.left -= int64(n)
	if err != nil {
		return n, fmt.Errorf("%w: %w", errWrite, err)
	}
	return n, nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
