package wine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkerRoundTrip(t *testing.T) {
	path := fakeBottle(t)
	want := Bottle{Key: "/Users/x/Games/Demo", Name: "Typhon-Demo", Path: path, Drive: "t", Games: "/Users/x/Games"}

	if err := writeMarker(path, want); err != nil {
		t.Fatalf("writeMarker: %v", err)
	}
	got, ok := readMarker(path)
	if !ok {
		t.Fatal("readMarker: not found")
	}
	if got.Key != want.Key || got.Drive != want.Drive || got.Games != want.Games || got.Name != want.Name {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// Path не хранится в метке: он и так известен из места, где метка лежит.
	if got.Path != path {
		t.Fatalf("Path = %q, want %q", got.Path, path)
	}
}

func TestReadMarkerAbsent(t *testing.T) {
	if _, ok := readMarker(fakeBottle(t)); ok {
		t.Fatal("readMarker on unmarked bottle: want not found")
	}
}

func TestReadMarkerCorrupt(t *testing.T) {
	path := fakeBottle(t)
	if err := os.WriteFile(filepath.Join(path, markerName), []byte("{ broken"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := readMarker(path); ok {
		t.Fatal("readMarker on corrupt marker: want not found")
	}
}

func TestBottleNameIsStableAndSafe(t *testing.T) {
	first := bottleName("/Users/x/Games/Some Game: Deluxe")
	second := bottleName("/Users/x/Games/Some Game: Deluxe")
	if first != second {
		t.Fatalf("bottleName is not stable: %q vs %q", first, second)
	}
	if filepath.Base(first) != first || first == "" {
		t.Fatalf("bottleName = %q, want a single safe path segment", first)
	}
	for _, bad := range []rune{'/', ':', '\\'} {
		for _, r := range first {
			if r == bad {
				t.Fatalf("bottleName = %q contains %q", first, string(bad))
			}
		}
	}
}

func TestBottleNameDistinguishesSamePrefix(t *testing.T) {
	first := bottleName("/Users/x/Games/Demo")
	second := bottleName("/Users/y/Games/Demo")
	if first == second {
		t.Fatalf("bottleName collides for different paths: %q", first)
	}
}

func TestToWindows(t *testing.T) {
	b := Bottle{Drive: "t", Games: "/Users/x/Games"}

	got, err := b.ToWindows("/Users/x/Games/Demo/game.exe")
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	if got != `T:\Demo\game.exe` {
		t.Fatalf("got %q, want %q", got, `T:\Demo\game.exe`)
	}
}

func TestToWindowsOutsideGames(t *testing.T) {
	b := Bottle{Drive: "t", Games: "/Users/x/Games"}

	if _, err := b.ToWindows("/tmp/installer.exe"); err == nil {
		t.Fatal("ToWindows outside the games folder: want error")
	}
}

func TestToNative(t *testing.T) {
	b := Bottle{Drive: "t", Games: "/Users/x/Games"}

	got, err := b.ToNative(`T:\Demo\game.exe`)
	if err != nil {
		t.Fatalf("ToNative: %v", err)
	}
	if got != "/Users/x/Games/Demo/game.exe" {
		t.Fatalf("got %q, want %q", got, "/Users/x/Games/Demo/game.exe")
	}
}

func TestToNativeOtherDrive(t *testing.T) {
	b := Bottle{Drive: "t", Games: "/Users/x/Games"}

	if _, err := b.ToNative(`C:\windows\system32\notepad.exe`); err == nil {
		t.Fatal("ToNative for another drive: want error")
	}
}
