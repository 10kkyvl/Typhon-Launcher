package steamlink

import (
	"bytes"
	"hash/crc32"
	"testing"
)

func sampleShortcut() Shortcut {
	return Shortcut{
		AppID:         AppID("game-1"),
		Name:          "Game One",
		Exe:           `C:\Games\One\one.exe`,
		StartDir:      `C:\Games\One`,
		Icon:          `C:\Games\One\one.exe`,
		LaunchOptions: "-fullscreen",
	}
}

func newEntry(index string, sc Shortcut) []byte {
	return cat(
		vMap(index),
		vInt("appid", sc.AppID),
		vStr("AppName", sc.Name),
		vStr("Exe", `"`+sc.Exe+`"`),
		vStr("StartDir", `"`+sc.StartDir+`"`),
		vStr("icon", sc.Icon),
		vStr("ShortcutPath", ""),
		vStr("LaunchOptions", sc.LaunchOptions),
		vInt("IsHidden", 0),
		vInt("AllowDesktopConfig", 1),
		vInt("AllowOverlay", 1),
		vInt("OpenVR", 0),
		vInt("Devkit", 0),
		vStr("DevkitGameID", ""),
		vInt("DevkitOverrideAppID", 0),
		vInt("LastPlayTime", 0),
		vStr("FlatpakAppID", ""),
		vMap("tags"), vEnd(),
		vEnd(),
	)
}

func TestAppID(t *testing.T) {
	got := AppID("abc")
	want := crc32.ChecksumIEEE([]byte("typhon:abc")) | 0x80000000
	if got != want {
		t.Fatalf("AppID = %d, want %d", got, want)
	}
	if got&0x80000000 == 0 {
		t.Fatal("high bit not set")
	}
	if AppID("abc") == AppID("abd") {
		t.Fatal("different games share an app id")
	}
}

func TestUpsertAndRemove(t *testing.T) {
	foreign0 := foreignEntry("0", 0x80000001, "Other")
	foreign1 := foreignEntry("1", 0x80000002, "Another")
	sc := sampleShortcut()

	t.Run("add to empty input", func(t *testing.T) {
		doc, err := parseShortcuts(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !doc.upsert(sc) {
			t.Fatal("upsert reported no change")
		}
		want := sampleFile(newEntry("0", sc))
		if got := doc.bytes(); !bytes.Equal(got, want) {
			t.Fatalf("got  %x\nwant %x", got, want)
		}
	})

	t.Run("add keeps foreign entries untouched", func(t *testing.T) {
		doc, err := parseShortcuts(sampleFile(foreign0, foreign1))
		if err != nil {
			t.Fatal(err)
		}
		doc.upsert(sc)
		want := sampleFile(foreign0, foreign1, newEntry("2", sc))
		if got := doc.bytes(); !bytes.Equal(got, want) {
			t.Fatalf("got  %x\nwant %x", got, want)
		}
	})

	t.Run("replace keeps play time and tags", func(t *testing.T) {
		existing := cat(
			vMap("1"),
			vInt("appid", sc.AppID),
			vStr("AppName", "Old name"),
			vStr("Exe", `"C:\old.exe"`),
			vStr("StartDir", `"C:\"`),
			vStr("icon", ""),
			vStr("LaunchOptions", ""),
			vU64("FutureField", 9),
			vInt("LastPlayTime", 1700000123),
			vMap("tags"), vStr("0", "mine"), vEnd(),
			vEnd(),
		)
		doc, err := parseShortcuts(sampleFile(foreign0, existing, foreign1))
		if err != nil {
			t.Fatal(err)
		}
		if !doc.upsert(sc) {
			t.Fatal("upsert reported no change")
		}
		want := sampleFile(foreign0, cat(
			vMap("1"),
			vInt("appid", sc.AppID),
			vStr("AppName", sc.Name),
			vStr("Exe", `"`+sc.Exe+`"`),
			vStr("StartDir", `"`+sc.StartDir+`"`),
			vStr("icon", sc.Icon),
			vStr("LaunchOptions", sc.LaunchOptions),
			vU64("FutureField", 9),
			vInt("LastPlayTime", 1700000123),
			vMap("tags"), vStr("0", "mine"), vEnd(),
			vEnd(),
		), foreign1)
		if got := doc.bytes(); !bytes.Equal(got, want) {
			t.Fatalf("got  %x\nwant %x", got, want)
		}
	})

	t.Run("identical entry is not a change", func(t *testing.T) {
		doc, err := parseShortcuts(sampleFile(newEntry("0", sc)))
		if err != nil {
			t.Fatal(err)
		}
		if doc.upsert(sc) {
			t.Fatal("upsert of an identical entry reported a change")
		}
	})

	t.Run("remove reindexes", func(t *testing.T) {
		doc, err := parseShortcuts(sampleFile(foreign0, newEntry("1", sc), foreignEntry("2", 0x80000003, "Third")))
		if err != nil {
			t.Fatal(err)
		}
		if !doc.remove(sc.AppID) {
			t.Fatal("remove reported no change")
		}
		want := sampleFile(foreign0, foreignEntry("1", 0x80000003, "Third"))
		if got := doc.bytes(); !bytes.Equal(got, want) {
			t.Fatalf("got  %x\nwant %x", got, want)
		}
	})

	t.Run("remove of a missing entry changes nothing", func(t *testing.T) {
		src := sampleFile(foreign0, foreign1)
		doc, err := parseShortcuts(src)
		if err != nil {
			t.Fatal(err)
		}
		if doc.remove(sc.AppID) {
			t.Fatal("remove reported a change")
		}
		if got := doc.bytes(); !bytes.Equal(got, src) {
			t.Fatalf("document changed: %x", got)
		}
	})

	t.Run("lookup ignores key case", func(t *testing.T) {
		upper := cat(vMap("shortcuts"), cat(vMap("0"), vInt("AppID", sc.AppID), vEnd()), vEnd(), vEnd())
		doc, err := parseShortcuts(upper)
		if err != nil {
			t.Fatal(err)
		}
		if !doc.has(sc.AppID) {
			t.Fatal("entry with upper-case key not found")
		}
	})
}

func TestParseShortcutsRejectsWrongShape(t *testing.T) {
	if _, err := parseShortcuts(cat(vStr("shortcuts", "x"), vEnd())); err == nil {
		t.Fatal("shortcuts as a string accepted")
	}
}
