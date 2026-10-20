package download

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

type fakePiece struct {
	st *fakeStorage
}

func (p fakePiece) ReadAt(b []byte, off int64) (int, error) { return len(b), nil }

func (p fakePiece) WriteAt(b []byte, off int64) (int, error) {
	if p.st.started != nil {
		p.st.started <- struct{}{}
		<-p.st.release
	}
	p.st.note("write")
	return len(b), nil
}

func (p fakePiece) MarkComplete() error {
	p.st.note("complete")
	return nil
}

func (p fakePiece) MarkNotComplete() error {
	p.st.note("not complete")
	return nil
}

func (p fakePiece) Completion() storage.Completion { return storage.Completion{} }

type fakeWriterToPiece struct {
	fakePiece
}

func (p fakeWriterToPiece) WriteTo(w io.Writer) (int64, error) { return 0, nil }

type fakeStorage struct {
	writerTo bool
	started  chan struct{}
	release  chan struct{}

	mu  sync.Mutex
	log []string
}

func (s *fakeStorage) note(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, event)
}

func (s *fakeStorage) events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.log...)
}

func (s *fakeStorage) OpenTorrent(context.Context, *metainfo.Info, metainfo.Hash) (storage.TorrentImpl, error) {
	return storage.TorrentImpl{Piece: func(metainfo.Piece) storage.PieceImpl {
		if s.writerTo {
			return fakeWriterToPiece{fakePiece{st: s}}
		}
		return fakePiece{st: s}
	}}, nil
}

func (s *fakeStorage) Close() error {
	s.note("closed")
	return nil
}

func openGatedPiece(t *testing.T, s *gatedStorage) storage.PieceImpl {
	t.Helper()
	ti, err := s.OpenTorrent(t.Context(), &metainfo.Info{}, metainfo.Hash{})
	if err != nil {
		t.Fatalf("OpenTorrent: %v", err)
	}
	return ti.Piece(metainfo.Piece{})
}

func TestGatedStorageRefusesChangesAfterClose(t *testing.T) {
	ops := []struct {
		name string
		run  func(storage.PieceImpl) error
	}{
		{"write", func(p storage.PieceImpl) error {
			_, err := p.WriteAt([]byte("x"), 1<<20)
			return err
		}},
		{"mark complete", func(p storage.PieceImpl) error { return p.MarkComplete() }},
		{"mark not complete", func(p storage.PieceImpl) error { return p.MarkNotComplete() }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			inner := &fakeStorage{}
			s := gateStorage(inner)
			piece := openGatedPiece(t, s)
			if err := op.run(piece); err != nil {
				t.Fatalf("before close: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := op.run(piece); !errors.Is(err, errStorageClosed) {
				t.Fatalf("after close err = %v, want errStorageClosed", err)
			}
			got := inner.events()
			if len(got) != 2 || got[1] != "closed" {
				t.Fatalf("inner saw %q, want one %s and then closed", got, op.name)
			}
		})
	}
}

func TestGatedStorageCloseWaitsForWriteInFlight(t *testing.T) {
	inner := &fakeStorage{started: make(chan struct{}), release: make(chan struct{})}
	s := gateStorage(inner)
	piece := openGatedPiece(t, s)

	wrote := make(chan error, 1)
	go func() {
		_, err := piece.WriteAt([]byte("x"), 0)
		wrote <- err
	}()
	<-inner.started

	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	for s.mu.TryRLock() {
		s.mu.RUnlock()
		select {
		case err := <-closed:
			t.Fatalf("Close returned (%v) while a write was in flight", err)
		default:
		}
		runtime.Gosched()
	}
	if got := inner.events(); len(got) != 0 {
		t.Fatalf("inner saw %q while the write was still in flight", got)
	}

	close(inner.release)
	if err := <-wrote; err != nil {
		t.Fatalf("write in flight: %v", err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := inner.events(); len(got) != 2 || got[0] != "write" || got[1] != "closed" {
		t.Fatalf("inner saw %q, want write then closed", got)
	}
}

func TestGatedStorageKeepsWriterTo(t *testing.T) {
	for _, writerTo := range []bool{true, false} {
		piece := openGatedPiece(t, gateStorage(&fakeStorage{writerTo: writerTo}))
		if _, ok := piece.(io.WriterTo); ok != writerTo {
			t.Fatalf("inner writerTo=%v, wrapped piece WriterTo=%v", writerTo, ok)
		}
	}
}

func TestGatedFileStorageDoesNotRecreateRemovedPart(t *testing.T) {
	dir := t.TempDir()
	const pieceLen = 16 << 10
	info := &metainfo.Info{Name: "disk.iso", Length: 2 * pieceLen, PieceLength: pieceLen, Pieces: make([]byte, 2*20)}
	s := gateStorage(storage.NewFileWithCompletion(dir, storage.NewMapPieceCompletion()))
	ti, err := s.OpenTorrent(t.Context(), info, metainfo.Hash{})
	if err != nil {
		t.Fatalf("OpenTorrent: %v", err)
	}
	piece := ti.Piece(info.Piece(1))
	part := filepath.Join(dir, info.Name+PartFileSuffix)
	if _, err := piece.WriteAt([]byte("chunk"), 0); err != nil {
		t.Fatalf("write before close: %v", err)
	}
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("part file after the first write: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(part); err != nil {
		t.Fatalf("remove part file: %v", err)
	}
	if _, err := piece.WriteAt([]byte("late"), 0); !errors.Is(err, errStorageClosed) {
		t.Fatalf("late write err = %v, want errStorageClosed", err)
	}
	if _, err := os.Stat(part); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("part file after the late write: %v, want it gone", err)
	}
}
