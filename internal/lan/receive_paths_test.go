package lan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

// A peer controls the file paths inside the torrent it serves. Paths that
// climb out of the receiving folder, or that Windows could not create, are
// refused before the storage opens, and the refusal is recorded so the
// transfer fails instead of hanging on GotInfo.
func TestReceiveStorageRefusesMetainfoWithUnsafePaths(t *testing.T) {
	cases := map[string]metainfo.Info{
		"escapes the folder":   {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"..", "escaped.bin"}, Length: 1}}},
		"windows separator":    {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{`..\escaped.bin`}, Length: 1}}},
		"reserved device name": {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"CON"}, Length: 1}}},
		"drive letter":         {Name: "Game", Files: []metainfo.FileInfo{{Path: []string{"C:", "x.bin"}, Length: 1}}},
	}
	for name, info := range cases {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), receivingPrefix+"x")
			if err := os.Mkdir(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			c := &lanClient{}
			st := guardedStorage{
				ClientImplCloser: receiveStorage(dest),
				refuse:           func(err error) { c.recordRefusal(dest, err) },
			}
			t.Cleanup(func() {
				if err := st.Close(); err != nil {
					t.Errorf("close storage: %v", err)
				}
			})
			_, err := st.OpenTorrent(context.Background(), &info, metainfo.Hash{})
			if !errors.Is(err, errUnsafeTorrentPaths) {
				t.Fatalf("OpenTorrent = %v, want %v", err, errUnsafeTorrentPaths)
			}
			if got := c.refusal(dest); !errors.Is(got, errUnsafeTorrentPaths) {
				t.Fatalf("recorded refusal = %v, want %v", got, errUnsafeTorrentPaths)
			}
			entries, err := os.ReadDir(filepath.Dir(dest))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("the refused torrent left files next to the receiving folder: %v", entries)
			}
			c.forgetRefusal(dest)
			if got := c.refusal(dest); got != nil {
				t.Fatalf("refusal survived forgetRefusal: %v", got)
			}
		})
	}
}

func TestReceiveStorageOpensAPlainLayoutAndRecordsNothing(t *testing.T) {
	dest := filepath.Join(t.TempDir(), receivingPrefix+"ok")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &lanClient{}
	st := guardedStorage{
		ClientImplCloser: receiveStorage(dest),
		refuse:           func(err error) { c.recordRefusal(dest, err) },
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close storage: %v", err)
		}
	})
	info := metainfo.Info{Name: "Game", PieceLength: 16384, Pieces: make([]byte, 20), Files: []metainfo.FileInfo{{Path: []string{"bin", "game.exe"}, Length: 1}}}
	impl, err := st.OpenTorrent(context.Background(), &info, metainfo.Hash{})
	if err != nil {
		t.Fatalf("OpenTorrent: %v", err)
	}
	if impl.Close != nil {
		if err := impl.Close(); err != nil {
			t.Errorf("close torrent storage: %v", err)
		}
	}
	if got := c.refusal(dest); got != nil {
		t.Fatalf("a plain layout was recorded as refused: %v", got)
	}
}
