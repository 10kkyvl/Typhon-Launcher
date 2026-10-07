package download

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func TestValidateInfo(t *testing.T) {
	file := func(path ...string) metainfo.FileInfo { return metainfo.FileInfo{Length: 1, Path: path} }
	folder := func(files ...metainfo.FileInfo) *metainfo.Info {
		return &metainfo.Info{Name: "Game", Files: files}
	}
	cases := []struct {
		name string
		info *metainfo.Info
		ok   bool
	}{
		{"a single file", &metainfo.Info{Name: "game.iso", Length: 10}, true},
		{"a folder with nested files", folder(file("a", "b.dat"), file("c.dat")), true},
		{"no info at all", nil, false},
		{"an empty name", &metainfo.Info{Name: "", Length: 10}, false},
		{"a name that is the parent directory", &metainfo.Info{Name: "..", Length: 10}, false},
		{"a name with a backslash", &metainfo.Info{Name: `a\b`, Length: 10}, false},
		{"a name with a drive", &metainfo.Info{Name: "C:", Length: 10}, false},
		{"a utf-8 name that is the parent directory", &metainfo.Info{Name: "Game", NameUtf8: "..", Length: 10}, false},
		{"a file that climbs out of the folder", folder(file("..", "x.dat")), false},
		{"a file below a climbing step", folder(file("a", "..", "..", "x.dat")), false},
		{"a file with an empty component", folder(file("a", "", "b.dat")), false},
		{"a file with a dot component", folder(file("a", ".", "b.dat")), false},
		{"a file with a backslash", folder(file(`a\b.dat`)), false},
		{"a file with an alternate data stream", folder(file("a.txt:stream")), false},
		{"a utf-8 file path that climbs out", folder(metainfo.FileInfo{Length: 1, Path: []string{"ok"}, PathUtf8: []string{"..", "x.dat"}}), false},
		{"a file of a folder without any path", folder(metainfo.FileInfo{Length: 1}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateInfo(c.info)
			switch {
			case c.ok && err != nil:
				t.Fatalf("a safe torrent was refused: %v", err)
			case !c.ok && err == nil:
				t.Fatal("an unsafe torrent was accepted")
			case !c.ok && !errors.Is(err, errBadPaths):
				t.Fatalf("error = %v, want errBadPaths", err)
			}
		})
	}
}

func flipByte(t *testing.T, dir, name string, offset int64) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err := f.ReadAt(b[:], offset); err != nil {
		t.Fatal(err)
	}
	b[0] ^= 0xff
	if _, err := f.WriteAt(b[:], offset); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTorrentReportsWhatIsReallyOnDisk(t *testing.T) {
	cases := []struct {
		name       string
		corrupt    func(t *testing.T, root string)
		wantHashed []bool
		wantA      int64
		partialB   bool
		wantPieces int
	}{
		{"intact data", func(*testing.T, string) {}, []bool{true, true, true}, 20000, false, 4},
		{"a piece inside the second file is damaged", func(t *testing.T, root string) {
			flipByte(t, root, "b.dat", 29000)
		}, []bool{true, false, true}, 20000, true, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mi, parent := buildTorrent(t, "Game", []tFile{{"a.dat", 20000}, {"b.dat", 30000}, {"z.empty", 0}})
			root := filepath.Join(parent, "Game")
			c.corrupt(t, root)
			cl := offlineClient(t)
			lt, err := cl.addMetainfo(mi, parent, storageOpts{inPlace: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(lt.drop)
			<-lt.t.GotInfo()

			if err := lt.verify(t.Context()); err != nil {
				t.Fatalf("verify: %v", err)
			}
			awaitPublishedCompletion(t, lt)

			if got := lt.filesHashed(); !slices.Equal(got, c.wantHashed) {
				t.Fatalf("filesHashed = %v, want %v: a file counts as hashed only when every piece under it is", got, c.wantHashed)
			}
			bytes := lt.fileBytes()
			if bytes[0] != c.wantA || bytes[2] != 0 {
				t.Fatalf("fileBytes = %v", bytes)
			}
			if c.partialB != (bytes[1] > 0 && bytes[1] < 30000) || (!c.partialB && bytes[1] != 30000) {
				t.Fatalf("bytes of the second file = %d", bytes[1])
			}
			if complete, total := lt.completePieces(); complete != c.wantPieces || total != 4 {
				t.Fatalf("pieces = %d/%d, want %d/4", complete, total, c.wantPieces)
			}
		})
	}
}

func TestLiveTorrentFilePathsFollowTheLayout(t *testing.T) {
	mi, parent := buildTorrent(t, "Game", []tFile{{"a.dat", 1000}, {"sub/b.dat", 2000}})
	root := filepath.Join(parent, "Game")
	cases := []struct {
		name string
		opts storageOpts
		dest string
		want []string
	}{
		{"nested keeps the torrent folder", storageOpts{inPlace: true}, parent,
			[]string{filepath.Join(parent, "Game", "a.dat"), filepath.Join(parent, "Game", "sub", "b.dat")}},
		{"flat drops it", storageOpts{inPlace: true, flat: true}, root,
			[]string{filepath.Join(root, "a.dat"), filepath.Join(root, "sub", "b.dat")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl := offlineClient(t)
			lt, err := cl.addMetainfo(mi, c.dest, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(lt.drop)
			<-lt.t.GotInfo()

			got := lt.filePaths(c.dest)

			if !slices.Equal(got, c.want) {
				t.Fatalf("paths = %v, want %v", got, c.want)
			}
			if err := lt.verify(t.Context()); err != nil {
				t.Fatal(err)
			}
			awaitPublishedCompletion(t, lt)
			if hashed := lt.filesHashed(); !slices.Equal(hashed, []bool{true, true}) {
				t.Fatalf("the engine does not see the data where filePaths says it is: hashed = %v", hashed)
			}
		})
	}
}

func awaitPublishedCompletion(t *testing.T, lt *liveTorrent) {
	t.Helper()
	waitUntil(t, "the engine to publish the completion of every piece", func() bool {
		for _, f := range lt.t.Files() {
			for _, s := range f.State() {
				if s.Marking || s.Checking {
					return false
				}
			}
		}
		return true
	})
}
