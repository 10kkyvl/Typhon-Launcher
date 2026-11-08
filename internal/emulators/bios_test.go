package emulators

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func sized(t *testing.T, path string, size int64) string {
	t.Helper()
	plant(t, path, nil)
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckBIOS(t *testing.T) {
	const program = "/Applications/DuckStation.app/Contents/MacOS/DuckStation"
	ps1, ps2 := mustPlatform(t, PS1), mustPlatform(t, PS2)

	t.Run("an emulator that never ran asks for the BIOS itself", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		if got := ps1.CheckBIOS(program, "darwin", env); got.Missing {
			t.Fatalf("no data folder: %+v", got)
		}
	})
	t.Run("a data folder without a bios folder is missing", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		root := filepath.Join(env.ConfigDir, "DuckStation")
		plant(t, filepath.Join(root, "settings.ini"), []byte("[BIOS]\nPatchFastBoot = false\n"))
		got := ps1.CheckBIOS(program, "darwin", env)
		if !got.Missing || got.Dir != filepath.Join(root, "bios") {
			t.Fatalf("CheckBIOS = %+v", got)
		}
	})
	t.Run("only files of a BIOS size count", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		bios := filepath.Join(env.ConfigDir, "DuckStation", "bios")
		sized(t, filepath.Join(bios, "readme.txt"), 100)
		sized(t, filepath.Join(bios, ".DS_Store"), 600<<10)
		if got := ps1.CheckBIOS(program, "darwin", env); !got.Missing {
			t.Fatalf("no image yet: %+v", got)
		}
		sized(t, filepath.Join(bios, "scph1001.bin"), 512<<10)
		if got := ps1.CheckBIOS(program, "darwin", env); got.Missing {
			t.Fatalf("with an image: %+v", got)
		}
	})
	t.Run("a PS1 image is not a PS2 BIOS", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		bios := filepath.Join(env.ConfigDir, "PCSX2", "bios")
		sized(t, filepath.Join(bios, "scph1001.bin"), 512<<10)
		if got := ps2.CheckBIOS("/Applications/PCSX2.app/Contents/MacOS/PCSX2", "darwin", env); !got.Missing {
			t.Fatalf("PS1 image for PS2: %+v", got)
		}
		sized(t, filepath.Join(bios, "scph39001.bin"), 4<<20)
		if got := ps2.CheckBIOS("/Applications/PCSX2.app/Contents/MacOS/PCSX2", "darwin", env); got.Missing {
			t.Fatalf("PS2 image: %+v", got)
		}
	})
	t.Run("the folder named in the settings is the one read", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		root := filepath.Join(env.ConfigDir, "PCSX2")
		elsewhere := t.TempDir()
		plant(t, filepath.Join(root, "inis", "PCSX2.ini"), []byte("[UI]\nBios = wrong\n[Folders]\n; comment\nBios = \""+elsewhere+"\"\n"))
		got := ps2.CheckBIOS("/x/PCSX2", "darwin", env)
		if !got.Missing || got.Dir != elsewhere {
			t.Fatalf("absolute folder: %+v", got)
		}
		sized(t, filepath.Join(elsewhere, "rom.bin"), 4<<20)
		if got := ps2.CheckBIOS("/x/PCSX2", "darwin", env); got.Missing {
			t.Fatalf("absolute folder with an image: %+v", got)
		}
		plant(t, filepath.Join(root, "inis", "PCSX2.ini"), []byte("\xef\xbb\xbf[folders]\nbios=dumps\n"))
		if got := ps2.CheckBIOS("/x/PCSX2", "darwin", env); got.Dir != filepath.Join(root, "dumps") {
			t.Fatalf("relative folder: %+v", got)
		}
	})
	t.Run("settings that cannot be read prove nothing", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		root := filepath.Join(env.ConfigDir, "DuckStation")
		if err := os.MkdirAll(filepath.Join(root, "settings.ini"), 0o700); err != nil {
			t.Fatal(err)
		}
		if got := ps1.CheckBIOS(program, "darwin", env); got.Missing || got.Dir != "" {
			t.Fatalf("unreadable settings: %+v", got)
		}
	})
	t.Run("a portable install keeps its data beside the program", func(t *testing.T) {
		dir := t.TempDir()
		exe := plant(t, filepath.Join(dir, "duckstation-qt.exe"), []byte("x"))
		plant(t, filepath.Join(dir, "portable.txt"), nil)
		env := BIOSEnv{CacheDir: t.TempDir()}
		sized(t, filepath.Join(env.CacheDir, "DuckStation", "bios", "scph1001.bin"), 512<<10)
		got := ps1.CheckBIOS(exe, "windows", env)
		if !got.Missing || got.Dir != filepath.Join(dir, "bios") {
			t.Fatalf("portable: %+v", got)
		}
	})
	t.Run("a link to an image counts", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		image := sized(t, filepath.Join(t.TempDir(), "scph1001.bin"), 512<<10)
		bios := filepath.Join(env.ConfigDir, "DuckStation", "bios")
		if err := os.MkdirAll(bios, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(image, filepath.Join(bios, "scph1001.bin")); err != nil {
			t.Skipf("no symlinks here: %v", err)
		}
		if got := ps1.CheckBIOS(program, "darwin", env); got.Missing {
			t.Fatalf("linked image: %+v", got)
		}
	})
	t.Run("consoles without a BIOS are never held back", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		for _, id := range []string{PSP, PS3, Vita, PS4} {
			if got := mustPlatform(t, id).CheckBIOS("/x/emu", "darwin", env); got.Missing || got.Dir != "" {
				t.Errorf("%s: %+v", id, got)
			}
			if _, ok := mustPlatform(t, id).BIOSDir("/x/emu", "darwin", env); ok {
				t.Errorf("%s has a BIOS folder", id)
			}
		}
	})
}

func TestBIOSRootsPerSystem(t *testing.T) {
	env := BIOSEnv{ConfigDir: "/cfg", CacheDir: `/local`, HomeDir: "/home/u", XDGDataDir: "/xdg"}
	cases := []struct {
		got, want []string
	}{
		{duckStationRoots("darwin", env), []string{"/cfg/DuckStation"}},
		{duckStationRoots("windows", env), []string{"/local/DuckStation", "/home/u/Documents/DuckStation"}},
		{duckStationRoots("linux", env), []string{"/xdg/duckstation"}},
		{duckStationRoots("linux", BIOSEnv{HomeDir: "/home/u"}), []string{"/home/u/.local/share/duckstation"}},
		{pcsx2Roots("darwin", env), []string{"/cfg/PCSX2"}},
		{pcsx2Roots("windows", env), []string{"/home/u/Documents/PCSX2"}},
		{pcsx2Roots("linux", env), []string{"/cfg/PCSX2"}},
		{duckStationRoots("darwin", BIOSEnv{ConfigDir: "relative"}), nil},
	}
	for i, c := range cases {
		want := make([]string, 0, len(c.want))
		for _, w := range c.want {
			want = append(want, filepath.FromSlash(w))
		}
		if !slices.Equal(c.got, want) && (len(c.got) != 0 || len(want) != 0) {
			t.Errorf("case %d: %q, want %q", i, c.got, want)
		}
	}
}

func TestIniValueReportsAnUnreadableLine(t *testing.T) {
	long := append([]byte("[BIOS]\nSearchDirectory = "), make([]byte, maxBIOSConfig+10)...)
	if _, _, err := iniValue(long, "BIOS", "SearchDirectory"); err == nil {
		t.Fatal("a line longer than the limit read as fine")
	}
}
