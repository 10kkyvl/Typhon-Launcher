package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

var errStubRead = errors.New("stub read failure")

type scriptedWriterTo struct {
	fakePiece
	n   int64
	err error
}

func (p scriptedWriterTo) WriteTo(io.Writer) (int64, error) { return p.n, p.err }

type scriptedStorage struct {
	fakeStorage
	n   int64
	err error
}

func (s *scriptedStorage) OpenTorrent(context.Context, *metainfo.Info, metainfo.Hash) (storage.TorrentImpl, error) {
	return storage.TorrentImpl{Piece: func(metainfo.Piece) storage.PieceImpl {
		return scriptedWriterTo{fakePiece: fakePiece{st: &s.fakeStorage}, n: s.n, err: s.err}
	}}, nil
}

func TestGatedPieceWriteToShortWrite(t *testing.T) {
	info := &metainfo.Info{Name: "f", Length: 40000, PieceLength: 16384, Pieces: make([]byte, 3*20)}
	tests := []struct {
		name    string
		piece   int
		n       int64
		err     error
		wantErr error
	}{
		{"whole piece then the overrun signal", 0, 16384, io.ErrShortWrite, nil},
		{"overrun signal wrapped", 1, 16384, fmt.Errorf("copy: %w", io.ErrShortWrite), nil},
		{"short last piece then the overrun signal", 2, 7232, io.ErrShortWrite, nil},
		{"clean write", 0, 16384, nil, nil},
		{"fewer bytes than the piece", 0, 16383, io.ErrShortWrite, io.ErrShortWrite},
		{"nothing written", 1, 0, io.ErrShortWrite, io.ErrShortWrite},
		{"more bytes than the piece", 2, 16384, io.ErrShortWrite, io.ErrShortWrite},
		{"other error after the whole piece", 0, 16384, errStubRead, errStubRead},
		{"unexpected eof", 0, 100, io.ErrUnexpectedEOF, io.ErrUnexpectedEOF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := gateStorage(&scriptedStorage{n: tc.n, err: tc.err})
			ti, err := s.OpenTorrent(t.Context(), info, metainfo.Hash{})
			if err != nil {
				t.Fatalf("OpenTorrent: %v", err)
			}
			to, ok := ti.Piece(info.Piece(tc.piece)).(io.WriterTo)
			if !ok {
				t.Fatal("wrapped piece lost io.WriterTo")
			}
			n, err := to.WriteTo(io.Discard)
			if n != tc.n {
				t.Fatalf("n = %d, want %d", n, tc.n)
			}
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

type capWriter struct{ left int }

func (w *capWriter) Write(p []byte) (int, error) {
	if len(p) > w.left {
		n := w.left
		w.left = 0
		return n, io.ErrShortWrite
	}
	w.left -= len(p)
	return len(p), nil
}

func TestFileStoragePieceInsideFileHashesWithoutError(t *testing.T) {
	const pieceLen = 32768
	const size = 3*pieceLen + 1696
	info := &metainfo.Info{Name: "f.bin", Length: size, PieceLength: pieceLen, Pieces: make([]byte, 4*20)}
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, info.Name), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := gateStorage(storage.NewFileWithCompletion(dir, storage.NewMapPieceCompletion()))
	ti, err := s.OpenTorrent(t.Context(), info, metainfo.Hash{})
	if err != nil {
		t.Fatalf("OpenTorrent: %v", err)
	}

	tests := []struct {
		name    string
		piece   int
		cap     int
		wantN   int64
		wantErr error
	}{
		{"first piece, ends inside the file", 0, 1 << 30, pieceLen, nil},
		{"middle piece, ends inside the file", 1, 1 << 30, pieceLen, nil},
		{"last piece, ends at the end of the file", 3, 1 << 30, 1696, nil},
		{"destination refuses bytes", 1, 1000, 1000, io.ErrShortWrite},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			to, ok := ti.Piece(info.Piece(tc.piece)).(io.WriterTo)
			if !ok {
				t.Fatal("wrapped piece lost io.WriterTo")
			}
			w := &capWriter{left: tc.cap}
			n, err := to.WriteTo(w)
			if n != tc.wantN {
				t.Fatalf("n = %d, want %d", n, tc.wantN)
			}
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
