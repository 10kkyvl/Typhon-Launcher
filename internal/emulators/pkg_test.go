package emulators

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pkgHeader lays out the start of a PS3 package as RPCS3 reads it.
func pkgHeader(kind byte, contentID string) []byte {
	out := make([]byte, 0xC0)
	copy(out, pkgMagic)
	out[pkgTypeOffset+1] = kind
	copy(out[contentIDStart:], contentID)
	return out
}

func TestPackageTitleID(t *testing.T) {
	root := t.TempDir()
	good := plant(t, filepath.Join(root, "apollo-ps3.pkg"), pkgHeader(1, "UP0001-NP0APOLLO_00-0000000000000000"))
	if id, err := PackageTitleID(good); err != nil || id != "NP0APOLLO" {
		t.Fatalf("PackageTitleID = %q, %v", id, err)
	}
	cases := map[string]struct {
		body []byte
		want error
	}{
		"not a package":   {[]byte("PK\x03\x04 a zip, not a package........................................................."), ErrPKGInvalid},
		"shorter":         {pkgHeader(1, "UP0001-NP0APOLLO_00")[:0x40], ErrPKGInvalid},
		"a PSP package":   {pkgHeader(2, "UP0001-NP0APOLLO_00-0000000000000000"), ErrPKGInvalid},
		"a broken id":     {pkgHeader(1, "UP0001-np0apo..._00-0000000000000000"), ErrTitleIDInvalid},
		"an empty header": {pkgHeader(1, ""), ErrTitleIDInvalid},
	}
	for name, c := range cases {
		path := plant(t, filepath.Join(root, name, "game.pkg"), c.body)
		if _, err := PackageTitleID(path); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := PackageTitleID(filepath.Join(root, "missing.pkg")); !errors.Is(err, ErrImageMissing) {
		t.Errorf("missing: %v", err)
	}
}

func TestInstalledPackageGame(t *testing.T) {
	ps3 := mustPlatform(t, PS3)
	const program = "/Applications/RPCS3.app/Contents/MacOS/rpcs3"

	t.Run("found in the default dev_hdd0", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		game := filepath.Join(env.ConfigDir, "rpcs3", "dev_hdd0", "game", "NP0APOLLO")
		plant(t, filepath.Join(game, "PARAM.SFO"), []byte("x"))
		if _, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); ok {
			t.Fatal("a folder without EBOOT.BIN counted as installed")
		}
		eboot := plant(t, filepath.Join(game, "USRDIR", "EBOOT.BIN"), []byte("x"))
		if got, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); !ok || got != eboot {
			t.Fatalf("InstalledPackageGame = %q, %v", got, ok)
		}
		if _, ok := ps3.InstalledPackageGame(program, "NPEB00001", "darwin", env); ok {
			t.Fatal("another title counted as installed")
		}
	})
	t.Run("dev_hdd0 moved by the mount table", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		root := filepath.Join(env.ConfigDir, "rpcs3")
		plant(t, filepath.Join(root, "config", "vfs.yml"), []byte("$(EmulatorDir): \"\"\n/dev_hdd0/: $(EmulatorDir)other_hdd/\n/dev_hdd1/: $(EmulatorDir)dev_hdd1/\n"))
		eboot := plant(t, filepath.Join(root, "other_hdd", "game", "NP0APOLLO", "USRDIR", "EBOOT.BIN"), []byte("x"))
		if got, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); !ok || got != eboot {
			t.Fatalf("relative mount: %q, %v", got, ok)
		}
		elsewhere := t.TempDir()
		plant(t, filepath.Join(root, "config", "vfs.yml"), []byte("/dev_hdd0: wrong\n/dev_hdd0/: '"+elsewhere+"/'\n"))
		eboot = plant(t, filepath.Join(elsewhere, "game", "NP0APOLLO", "USRDIR", "EBOOT.BIN"), []byte("x"))
		if got, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); !ok || got != eboot {
			t.Fatalf("absolute mount: %q, %v", got, ok)
		}
		plant(t, filepath.Join(root, "config", "vfs.yml"), []byte("/dev_hdd0/: relative/path/\n"))
		if _, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); ok {
			t.Fatal("a relative mount counted")
		}
	})
	t.Run("a mount table that cannot be read leaves the game unknown", func(t *testing.T) {
		env := BIOSEnv{ConfigDir: t.TempDir()}
		root := filepath.Join(env.ConfigDir, "rpcs3")
		plant(t, filepath.Join(root, "dev_hdd0", "game", "NP0APOLLO", "USRDIR", "EBOOT.BIN"), []byte("x"))
		if err := os.MkdirAll(filepath.Join(root, "config", "vfs.yml"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, ok := ps3.InstalledPackageGame(program, "NP0APOLLO", "darwin", env); ok {
			t.Fatal("an unreadable mount table counted")
		}
	})
	t.Run("on Windows the data lies beside the program", func(t *testing.T) {
		dir := t.TempDir()
		exe := plant(t, filepath.Join(dir, "rpcs3.exe"), []byte("x"))
		eboot := plant(t, filepath.Join(dir, "dev_hdd0", "game", "NP0APOLLO", "USRDIR", "EBOOT.BIN"), []byte("x"))
		if got, ok := ps3.InstalledPackageGame(exe, "NP0APOLLO", "windows", BIOSEnv{}); !ok || got != eboot {
			t.Fatalf("windows: %q, %v", got, ok)
		}
	})
	t.Run("other consoles install nothing", func(t *testing.T) {
		for _, id := range []string{PS1, PS2, PSP, Vita, PS4} {
			p := mustPlatform(t, id)
			if p.AcceptsPackage("game.pkg") {
				t.Errorf("%s takes packages", id)
			}
			if _, ok := p.InstalledPackageGame(program, "NP0APOLLO", "darwin", BIOSEnv{ConfigDir: t.TempDir()}); ok {
				t.Errorf("%s found an installed package", id)
			}
		}
	})
}
