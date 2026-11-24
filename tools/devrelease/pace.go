package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	minPaceChunk = 4 << 10
	maxPaceChunk = 256 << 10
)

type waitFunc func(ctx context.Context, d time.Duration) error

func sleepCtx(ctx context.Context, d time.Duration) error {
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

// pacedWriter spreads a response over time so the launcher's progress bar,
// cancel and resume can be watched: every chunk is flushed to the client and
// followed by the pause its size is worth at the given rate. The rate comes
// from the whole artifact, so a ranged resume keeps the same speed.
type pacedWriter struct {
	http.ResponseWriter
	ctx   context.Context
	rate  int64
	chunk int
	wait  waitFunc
}

func newPacedWriter(ctx context.Context, w http.ResponseWriter, size int64, seconds int, wait waitFunc) http.ResponseWriter {
	if seconds <= 0 || size <= 0 {
		return w
	}
	rate := max((size+int64(seconds)-1)/int64(seconds), 1)
	chunk := int(min(max(rate/20, minPaceChunk), maxPaceChunk))
	return &pacedWriter{ResponseWriter: w, ctx: ctx, rate: rate, chunk: chunk, wait: wait}
}

func (p *pacedWriter) Write(b []byte) (int, error) {
	written := 0
	for len(b) > 0 {
		n := min(len(b), p.chunk)
		m, err := p.ResponseWriter.Write(b[:n])
		written += m
		if err != nil {
			return written, err
		}
		if err := http.NewResponseController(p.ResponseWriter).Flush(); err != nil {
			return written, fmt.Errorf("flush paced chunk: %w", err)
		}
		if err := p.wait(p.ctx, time.Duration(int64(n)*int64(time.Second)/p.rate)); err != nil {
			return written, err
		}
		b = b[n:]
	}
	return written, nil
}

func (p *pacedWriter) Unwrap() http.ResponseWriter {
	return p.ResponseWriter
}
