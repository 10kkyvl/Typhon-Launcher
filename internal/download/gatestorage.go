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
type gatedStorage struct {
	inner  storage.ClientImplCloser
	mu     sync.RWMutex
	closed bool
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
		t.Piece = func(p metainfo.Piece) storage.PieceImpl { return s.wrap(piece(p)) }
	}
	if piece := t.PieceWithHash; piece != nil {
		t.PieceWithHash = func(p metainfo.Piece, hash g.Option[[]byte]) storage.PieceImpl { return s.wrap(piece(p, hash)) }
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

func (s *gatedStorage) wrap(p storage.PieceImpl) storage.PieceImpl {
	gp := gatedPiece{PieceImpl: p, gate: s}
	if w, ok := p.(io.WriterTo); ok {
		return gatedWriterToPiece{gatedPiece: gp, to: w}
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
	return p.PieceImpl.MarkComplete()
}

func (p gatedPiece) MarkNotComplete() error {
	if err := p.gate.enter(); err != nil {
		return err
	}
	defer p.gate.leave()
	return p.PieceImpl.MarkNotComplete()
}

type gatedWriterToPiece struct {
	gatedPiece
	to io.WriterTo
}

func (p gatedWriterToPiece) WriteTo(w io.Writer) (int64, error) { return p.to.WriteTo(w) }
