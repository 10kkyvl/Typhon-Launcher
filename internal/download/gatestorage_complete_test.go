package download

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

func TestGatedStorageCompletesPiecesOfOneFileWithoutPromotionErrors(t *testing.T) {
	const pieceLen = 16 << 10
	const pieces = 32
	const rounds = 40
	info := &metainfo.Info{Name: "bundle.bin", Length: pieces * pieceLen, PieceLength: pieceLen, Pieces: make([]byte, pieces*20)}

	for round := range rounds {
		dir := t.TempDir()
		s := gateStorage(storage.NewFileWithCompletion(dir, storage.NewMapPieceCompletion()))
		ti, err := s.OpenTorrent(t.Context(), info, metainfo.Hash{byte(round)})
		if err != nil {
			t.Fatalf("round %d: OpenTorrent: %v", round, err)
		}
		for i := range pieces {
			if _, err := ti.Piece(info.Piece(i)).WriteAt(make([]byte, pieceLen), 0); err != nil {
				t.Fatalf("round %d: write piece %d: %v", round, i, err)
			}
		}

		var wg sync.WaitGroup
		errs := make([]error, pieces)
		for i := range pieces {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = ti.Piece(info.Piece(i)).MarkComplete()
			}()
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: MarkComplete of piece %d: %v", round, i, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, info.Name)); err != nil {
			t.Fatalf("round %d: the finished file was not promoted: %v", round, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("round %d: Close: %v", round, err)
		}
	}
}
