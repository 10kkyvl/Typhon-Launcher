package download

import (
	"context"
	"errors"
	"io"
	"sync"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

var errStorageClosed = errors.New("хранилище загрузки закрыто")

// gatedStorage stops every write to the files once it is closed. The engine
// writes chunks without holding its storage lock, so a write that began
// before Torrent.Drop can land after it and, through O_CREATE, bring back a
// .part file that the removal has already deleted. Close waits for the
// writes in flight and refuses the ones that come later.
//
// It also runs MarkComplete and MarkNotComplete one at a time. Each of them
// fsyncs the files of its piece through a handle opened without
// FILE_SHARE_DELETE, outside the engine's per-file lock, and the one that
// finishes a file then renames its .part. On Windows that rename fails with a
// sharing violation whenever a neighbouring piece of the same file is still in
// its fsync, and the engine logs the failure and never retries it.
type gatedStorage struct {
	inner  storage.ClientImplCloser
	mu     sync.RWMutex
	closed bool
	marks  sync.Mutex
}

func gateStorage(inner storage.ClientImplCloser) *gatedStorage {
	return &gatedStorage{inner: inner}
}

func (s *gatedStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (storage.TorrentImpl, error) {
	t, err := s.inner.OpenTorrent(ctx, info, infoHash)
	if err != nil {
		return t, err
	}
	if piece := t.Piece; piece != nil {
		t.Piece = func(p metainfo.Piece) storage.PieceImpl { return s.wrap(p, piece(p)) }
	}
	if piece := t.PieceWithHash; piece != nil {
		t.PieceWithHash = func(p metainfo.Piece, hash g.Option[[]byte]) storage.PieceImpl { return s.wrap(p, piece(p, hash)) }
	}
	return t, nil
}

func (s *gatedStorage) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.inner.Close()
}

func (s *gatedStorage) enter() error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return errStorageClosed
	}
	return nil
}

func (s *gatedStorage) leave() { s.mu.RUnlock() }

func (s *gatedStorage) wrap(meta metainfo.Piece, p storage.PieceImpl) storage.PieceImpl {
	gp := gatedPiece{PieceImpl: p, gate: s}
	if w, ok := p.(io.WriterTo); ok {
		return gatedWriterToPiece{gatedPiece: gp, to: w, meta: meta}
	}
	return gp
}

type gatedPiece struct {
	storage.PieceImpl
	gate *gatedStorage
}

func (p gatedPiece) WriteAt(b []byte, off int64) (int, error) {
	if err := p.gate.enter(); err != nil {
		return 0, err
	}
	defer p.gate.leave()
	return p.PieceImpl.WriteAt(b, off)
}

func (p gatedPiece) MarkComplete() error {
	if err := p.gate.enter(); err != nil {
		return err
	}
	defer p.gate.leave()
	p.gate.marks.Lock()
	defer p.gate.marks.Unlock()
	return p.PieceImpl.MarkComplete()
}

func (p gatedPiece) MarkNotComplete() error {
	if err := p.gate.enter(); err != nil {
		return err
	}
	defer p.gate.leave()
	p.gate.marks.Lock()
	defer p.gate.marks.Unlock()
	return p.PieceImpl.MarkNotComplete()
}

type gatedWriterToPiece struct {
	gatedPiece
	to   io.WriterTo
	meta metainfo.Piece
}

// The file storage copies a piece through a writer that refuses bytes past the
// end of the piece's extent, and io.Copy only finds that end by reading one
// buffer too far, so every piece that ends inside a file comes back with
// io.ErrShortWrite after all of its bytes were written. The engine reads that
// as a storage failure: it logs a warning per piece, skips banning the peers
// that sent bad blocks and does not count a failed hash against them. The error
// is dropped only when the whole piece was written.
func (p gatedWriterToPiece) WriteTo(w io.Writer) (int64, error) {
	n, err := p.to.WriteTo(w)
	if errors.Is(err, io.ErrShortWrite) && n == p.meta.Length() {
		return n, nil
	}
	return n, err
}
