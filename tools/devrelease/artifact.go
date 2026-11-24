package main

import (
	"context"
	"io"
)

// artifactReader is the artifact as one download request sees it. The size
// and hash faults are fixed when the request starts: a short file (one byte
// missing) or the right length with the last byte flipped, so the transfer
// runs to the end before the launcher finds out. The stall is read live, so
// switching it on mid-download freezes a running transfer: no byte past the
// middle of the file (or past where it already is) goes out while it lasts.
type artifactReader struct {
	ctx     context.Context
	r       io.ReadSeeker
	pos     int64
	stallAt int64
	flipAt  int64
	ctl     *control
}

func newArtifactReader(ctx context.Context, f io.ReaderAt, size int64, mode failMode, ctl *control) *artifactReader {
	length, flipAt := size, int64(-1)
	switch mode {
	case failSize:
		length = max(size-1, 0)
	case failHash:
		flipAt = size - 1
	}
	return &artifactReader{ctx: ctx, r: io.NewSectionReader(f, 0, length), stallAt: size / 2, flipAt: flipAt, ctl: ctl}
}

func (a *artifactReader) Read(p []byte) (int, error) {
	if a.ctl.current() == failStall {
		if a.pos >= a.stallAt {
			if err := a.ctl.holdWhileStalled(a.ctx); err != nil {
				return 0, err
			}
		} else if rest := a.stallAt - a.pos; int64(len(p)) > rest {
			p = p[:rest]
		}
	}
	n, err := a.r.Read(p)
	if a.flipAt >= a.pos && a.flipAt < a.pos+int64(n) {
		p[a.flipAt-a.pos] ^= 0xff
	}
	a.pos += int64(n)
	return n, err
}

func (a *artifactReader) Seek(offset int64, whence int) (int64, error) {
	pos, err := a.r.Seek(offset, whence)
	if err != nil {
		return pos, err
	}
	a.pos = pos
	return pos, nil
}
