package emulators

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindStorage(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	quoted := strings.ReplaceAll(root, `\`, `\\`)
	cases := []struct {
		name   string
		config string
		want   Storage
	}{
		{"no config is the default", "", Storage{Root: home, Known: true}},
		{"a config without the key", "log-level: 0\n", Storage{Root: home, Known: true}},
		{"an empty key", "pref-path:\n", Storage{Root: home, Known: true}},
		{"a plain path", "log-level: 0\npref-path: " + root + "\ncpu-opt: true\n", Storage{Root: root, Known: true}},
		{"windows line ends", "pref-path: " + root + "\r\n", Storage{Root: root, Known: true}},
		{"a byte order mark", utf8BOM + "pref-path: " + root + "\n", Storage{Root: root, Known: true}},
		{"single quotes", "pref-path: '" + root + "'\n", Storage{Root: root, Known: true}},
		{"double quotes", `pref-path: "` + quoted + "\"\n", Storage{Root: root, Known: true}},
		{"a nested key is another key", "gui:\n  pref-path: " + root + "\n", Storage{Root: home, Known: true}},

		{"a relative path", "pref-path: games/vita\n", Storage{}},
		{"a null", "pref-path: ~\n", Storage{}},
		{"unfinished quotes", "pref-path: \"" + quoted + "\n", Storage{}},
		{"unfinished single quotes", "pref-path: '" + root + "\n", Storage{}},
		{"an unknown escape", `pref-path: "\x41"` + "\n", Storage{}},
		{"a comment after the value", "pref-path: " + root + " # mine\n", Storage{}},
		{"two keys", "pref-path: " + root + "\npref-path: " + root + "\n", Storage{}},
		{"not text", "pref-path: \xff\xfe\n", Storage{}},
		{"oversized", "pref-path: " + root + "\n" + strings.Repeat("#", maxStorageConfig), Storage{}},
	}
	vita := mustPlatform(t, Vita)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.config != "" {
				plant(t, filepath.Join(dir, storageConfig), []byte(c.config))
			}
			got := vita.FindStorage(filepath.Join(dir, "Vita3K.exe"), func() (string, error) { return home, nil })
			if got != c.want {
				t.Fatalf("FindStorage = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestFindStorageNeverGuessesTheDefault(t *testing.T) {
	vita := mustPlatform(t, Vita)
	home := func() (string, error) { return t.TempDir(), nil }

	t.Run("a config that cannot be read", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, storageConfig), 0o700); err != nil {
			t.Fatal(err)
		}
		if got := vita.FindStorage(filepath.Join(dir, "Vita3K.exe"), home); got.Known {
			t.Fatalf("FindStorage = %+v", got)
		}
	})
	t.Run("a portable install", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, portableMark), 0o700); err != nil {
			t.Fatal(err)
		}
		if got := vita.FindStorage(filepath.Join(dir, "Vita3K.exe"), home); got.Known {
			t.Fatalf("FindStorage = %+v", got)
		}
	})
	t.Run("no default location", func(t *testing.T) {
		got := vita.FindStorage(filepath.Join(t.TempDir(), "Vita3K.exe"), func() (string, error) { return "", errors.New("no home") })
		if got.Known {
			t.Fatalf("FindStorage = %+v", got)
		}
	})
	t.Run("a relative default location", func(t *testing.T) {
		got := vita.FindStorage(filepath.Join(t.TempDir(), "Vita3K.exe"), func() (string, error) { return "Vita3K", nil })
		if got.Known {
			t.Fatalf("FindStorage = %+v", got)
		}
	})
	t.Run("another console", func(t *testing.T) {
		if got := mustPlatform(t, PS4).FindStorage(filepath.Join(t.TempDir(), "shadPS4.exe"), home); got.Known {
			t.Fatalf("FindStorage = %+v", got)
		}
	})
}

func TestPlace(t *testing.T) {
	vita := mustPlatform(t, Vita)
	root, dump := t.TempDir(), t.TempDir()
	known := Storage{Root: root, Known: true}
	in := func(base string, parts ...string) string {
		return plant(t, filepath.Join(append(append([]string{base}, parts...), "eboot.bin")...), []byte("x"))
	}
	cases := []struct {
		name    string
		image   string
		storage Storage
		want    Placement
	}{
		{"own folder", in(root, "ux0", "app", "PCSE00001"), known, PlaceInstalled},
		{"inside its own folder", in(root, "ux0", "app", "PCSE00001", "a", "b"), known, PlaceInstalled},
		{"patch", in(root, "ux0", "patch", "PCSE00001"), known, PlaceData},
		{"add-on", in(root, "ux0", "addcont", "PCSE00001", "x"), known, PlaceData},
		{"a folder of another id", in(root, "ux0", "app", "PCSE00002"), known, PlaceOutside},
		{"a dump", in(dump, "ux0", "app", "PCSE00001"), known, PlaceOutside},
		{"a dump of a patch", in(dump, "ux0", "patch", "PCSE00001"), known, PlaceOutside},
		{"a dump, storage unknown", in(dump, "ux0", "app", "PCSE00001"), Storage{}, PlaceInstalled},
		{"another name, storage unknown", in(dump, "ux0", "app", "other"), Storage{}, PlaceInstalled},
		{"a patch, storage unknown", in(dump, "ux0", "patch", "PCSE00001"), Storage{}, PlaceData},
		{"a plain folder, storage unknown", in(dump, "plain"), Storage{}, PlaceOutside},
		{"the file is gone, so the name decides", filepath.Join(dump, "gone", "ux0", "app", "x", "eboot.bin"), known, PlaceInstalled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := vita.Place(c.image, "PCSE00001", c.storage); got != c.want {
				t.Fatalf("Place = %v, want %v", got, c.want)
			}
		})
	}
	if got := mustPlatform(t, PS4).Place(in(root, "ux0", "app", "CUSA00001"), "CUSA00001", known); got != PlaceOutside {
		t.Fatalf("a console that installs nothing: %v", got)
	}
}
