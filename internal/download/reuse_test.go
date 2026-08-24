package download

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func infoWith(name string, files ...metainfo.FileInfo) *metainfo.Info {
	return &metainfo.Info{Name: name, PieceLength: 1 << 18, Files: files}
}

func file(size int64, path ...string) metainfo.FileInfo {
	return metainfo.FileInfo{Length: size, Path: path}
}

func TestClassifyLayout(t *testing.T) {
	direct := infoWith("Game",
		file(40<<20, "Game.exe"),
		file(4<<20, "d3d11.dll"),
		file(900<<20, "data", "pak0.pak"),
		file(700<<20, "data", "pak1.pak"),
		file(120<<20, "data", "audio.bank"),
	)
	if got := ClassifyLayout(direct); got != LayoutDirectFiles {
		t.Fatalf("layout = %q, want %q", got, LayoutDirectFiles)
	}

	repack := infoWith("Game Repack",
		file(2<<20, "setup.exe"),
		file(8<<30, "data1.bin"),
		file(8<<30, "data2.bin"),
	)
	if got := ClassifyLayout(repack); got != LayoutArchivePackage {
		t.Fatalf("layout = %q, want %q", got, LayoutArchivePackage)
	}

	installer := infoWith("Game Installer",
		file(6<<30, "setup.exe"),
		file(1<<10, "readme.txt"),
	)
	if got := ClassifyLayout(installer); got != LayoutInstallerPackage {
		t.Fatalf("layout = %q, want %q", got, LayoutInstallerPackage)
	}

	if got := ClassifyLayout(infoWith("Empty")); got != LayoutUnknown {
		t.Fatalf("layout = %q, want %q", got, LayoutUnknown)
	}
}

func TestRelativePaths(t *testing.T) {
	info := infoWith("Game", file(1, "bin", "game.exe"), file(2, "data.pak"))
	nested := relativePaths(info, false)
	if nested[0] != filepath.Join("Game", "bin", "game.exe") || nested[1] != filepath.Join("Game", "data.pak") {
		t.Fatalf("nested = %v", nested)
	}
	flat := relativePaths(info, true)
	if flat[0] != filepath.Join("bin", "game.exe") || flat[1] != "data.pak" {
		t.Fatalf("flat = %v", flat)
	}

	single := &metainfo.Info{Name: "game.exe", Length: 10, PieceLength: 1 << 18}
	if supportsFlat(single) {
		t.Fatal("single file torrents cannot be mapped flat")
	}
	if got := relativePaths(single, false); len(got) != 1 || got[0] != "game.exe" {
		t.Fatalf("single = %v", got)
	}
}

func TestChooseMapping(t *testing.T) {
	info := infoWith("Game", file(4, "bin", "game.exe"), file(6, "data.pak"))

	flatRoot := t.TempDir()
	write(t, filepath.Join(flatRoot, "bin", "game.exe"), 4)
	write(t, filepath.Join(flatRoot, "data.pak"), 6)
	if got := chooseMapping(info, flatRoot); !got.flat || got.present != 2 || got.bytes != 10 {
		t.Fatalf("flat root mapping = %+v", got)
	}

	nestedRoot := t.TempDir()
	write(t, filepath.Join(nestedRoot, "Game", "bin", "game.exe"), 4)
	write(t, filepath.Join(nestedRoot, "Game", "data.pak"), 6)
	if got := chooseMapping(info, nestedRoot); got.flat || got.present != 2 || got.bytes != 10 {
		t.Fatalf("nested root mapping = %+v", got)
	}
}

// A directory the torrent does not describe must be reported as "found
// nothing", never as a layout guess that then declares every file missing.
func TestChooseMappingUnrelatedDirectory(t *testing.T) {
	info := infoWith("Game", file(4, "bin", "game.exe"), file(6, "data.pak"))

	cases := map[string]func(root string){
		"empty": func(string) {},
		"unpacked elsewhere": func(root string) {
			write(t, filepath.Join(root, "Some Game", "bin", "game.exe"), 4)
			write(t, filepath.Join(root, "Some Game", "data.pak"), 6)
		},
		"only unrelated files": func(root string) {
			write(t, filepath.Join(root, "readme.txt"), 12)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			setup(root)
			got := chooseMapping(info, root)
			if got.present != 0 || got.bytes != 0 {
				t.Fatalf("mapping = %+v, want nothing found", got)
			}
		})
	}
}

// A file that exists but no longer matches its recorded size is damage, and
// must keep the mapping applicable so that the damage is actually reported.
func TestInspectMappingSeparatesDamageFromAbsence(t *testing.T) {
	info := &metainfo.Info{Name: "game.rar", Length: 10, PieceLength: 1 << 18}

	root := t.TempDir()
	write(t, filepath.Join(root, "game.rar"), 3)
	got := inspectMapping(root, info, false)
	if got.present != 1 || got.bytes != 0 {
		t.Fatalf("truncated file mapping = %+v, want present with no matching bytes", got)
	}

	if got := inspectMapping(t.TempDir(), info, false); got.present != 0 {
		t.Fatalf("absent file mapping = %+v, want nothing found", got)
	}
}

func TestPieceFileIndex(t *testing.T) {
	info := &metainfo.Info{
		Name:        "Game",
		PieceLength: 1 << 10,
		Files:       []metainfo.FileInfo{file(2<<10, "a.bin"), file(3<<10, "b.bin")},
	}
	owners := pieceFileIndex(info)
	if len(owners) != 5 {
		t.Fatalf("pieces = %d, want 5", len(owners))
	}
	want := []int{0, 0, 1, 1, 1}
	for i, owner := range owners {
		if owner != want[i] {
			t.Fatalf("piece %d owned by file %d, want %d", i, owner, want[i])
		}
	}
}

func write(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}
