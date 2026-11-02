package emulators

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"typhon/internal/emulators/sfotest"
)

func mustPlatform(t *testing.T, id string) Platform {
	t.Helper()
	p, ok := Lookup(id)
	if !ok {
		t.Fatalf("no platform %q", id)
	}
	return p
}

func plant(t *testing.T, path string, body []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func gameSFO(title, category string) []byte {
	return sfotest.Bytes(map[string]string{"TITLE_ID": title, "CATEGORY": category, "TITLE": "Игра"})
}

func writeZip(t *testing.T, path string, entries map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return plant(t, path, buf.Bytes())
}

func TestAcceptsImageOnNextGenConsoles(t *testing.T) {
	cases := []struct {
		platform string
		path     string
		want     bool
	}{
		{PS3, filepath.Join("g", "USRDIR", "EBOOT.BIN"), true},
		{PS3, filepath.Join("g", "usrdir", "eboot.bin"), true},
		{PS3, filepath.Join("g", "EBOOT.BIN"), false},
		{PS3, filepath.Join("g", "USRDIR", "game.bin"), false},
		{PS3, "game.iso", false},
		{PS3, "game.pkg", false},
		{PS4, filepath.Join("g", "eboot.bin"), true},
		{PS4, filepath.Join("g", "EBOOT.BIN"), true},
		{PS4, filepath.Join("g", "game.bin"), false},
		{PS4, "game.iso", false},
		{Vita, "game.vpk", true},
		{Vita, "game.VPK", true},
		{Vita, filepath.Join("g", "eboot.bin"), true},
		{Vita, filepath.Join("g", "game.bin"), false},
		{Vita, "game.iso", false},
		{PS2, filepath.Join("g", "USRDIR", "EBOOT.BIN"), true},
		{PS1, filepath.Join("g", "track.bin"), true},
	}
	for _, c := range cases {
		t.Run(c.platform+" "+c.path, func(t *testing.T) {
			if got := mustPlatform(t, c.platform).AcceptsImage(c.path); got != c.want {
				t.Fatalf("AcceptsImage = %v, want %v", got, c.want)
			}
		})
	}
}

func TestManualOnlyStaysOffTheOlderConsoles(t *testing.T) {
	for _, id := range []string{PS1, PS2, PSP} {
		p := mustPlatform(t, id)
		if p.ManualOnly("game.iso") || p.ManualOnly("game.pkg") {
			t.Fatalf("%s: a form it plays is refused", id)
		}
	}
	for _, id := range []string{PS3, Vita, PS4} {
		p := mustPlatform(t, id)
		if !p.ManualOnly("game.ISO") || !p.ManualOnly("game.pkg") {
			t.Fatalf("%s: no refusal for a disc image or a package", id)
		}
	}
}

func TestGameFolder(t *testing.T) {
	sfo := gameSFO("BLES00001", "DG")
	cases := []struct {
		name     string
		platform string
		files    []string
		image    string
		want     string
		err      error
	}{
		{name: "disc", platform: PS3, files: []string{"Game/PS3_DISC.SFB", "Game/PS3_GAME/PARAM.SFO"},
			image: "Game/PS3_GAME/USRDIR/EBOOT.BIN", want: "Game"},
		{name: "a folder without the mark of a disc stays out of the game", platform: PS3,
			files: []string{"Downloads/PS3_GAME/PARAM.SFO", "Downloads/other.mkv"},
			image: "Downloads/PS3_GAME/USRDIR/EBOOT.BIN", want: "Downloads/PS3_GAME"},
		{name: "title folder", platform: PS3, files: []string{"BLES00001/PARAM.SFO"},
			image: "BLES00001/USRDIR/EBOOT.BIN", want: "BLES00001"},
		{name: "another case", platform: PS3, files: []string{"BLES00001/param.sfo"},
			image: "BLES00001/usrdir/eboot.bin", want: "BLES00001"},
		{name: "ps3 without the proof", platform: PS3, image: "Game/USRDIR/EBOOT.BIN", err: ErrImageIncomplete},
		{name: "ps4", platform: PS4, files: []string{"CUSA00001/sce_sys/param.sfo"},
			image: "CUSA00001/eboot.bin", want: "CUSA00001"},
		{name: "ps4 without the proof", platform: PS4, image: "CUSA00001/eboot.bin", err: ErrImageIncomplete},
		{name: "proof that is a folder", platform: PS4, files: []string{"G/sce_sys/param.sfo/x"},
			image: "G/eboot.bin", err: ErrImageIncomplete},
		{name: "vita", platform: Vita, files: []string{"PCSE00001/sce_sys/param.sfo"},
			image: "PCSE00001/eboot.bin", want: "PCSE00001"},
		{name: "vpk stays a file", platform: Vita, image: "Games/game.vpk", want: "Games"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range c.files {
				plant(t, filepath.Join(root, filepath.FromSlash(name)), sfo)
			}
			image := plant(t, filepath.Join(root, filepath.FromSlash(c.image)), []byte("x"))
			got, err := mustPlatform(t, c.platform).GameFolder(image)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("GameFolder = %q, %v, want %v", got, err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(root, filepath.FromSlash(c.want)); got != want {
				t.Fatalf("GameFolder = %q, want %q", got, want)
			}
		})
	}
}

func TestGameFolderDoesNotTakeALinkForProof(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	plant(t, filepath.Join(outside, "param.sfo"), gameSFO("CUSA00001", "gd"))
	image := plant(t, filepath.Join(root, "eboot.bin"), []byte("x"))
	link := filepath.Join(root, "sce_sys")
	if runtime.GOOS == "windows" {
		//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Fatalf("mklink: %v: %s", err, out)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := mustPlatform(t, PS4).GameFolder(image); !errors.Is(err, ErrImageIncomplete) {
		t.Fatalf("GameFolder = %v", err)
	}
}

func TestParseSFO(t *testing.T) {
	good := gameSFO("PCSE00001", "gd")
	info, err := parseSFO(good)
	if err != nil || info.TitleID != "PCSE00001" || info.Category != "gd" {
		t.Fatalf("parseSFO = %+v, %v", info, err)
	}
	mutate := func(edit func(b []byte) []byte) []byte { return edit(bytes.Clone(good)) }
	put32 := func(at int, value uint32) []byte {
		return mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[at:], value)
			return b
		})
	}
	put16 := func(at int, value uint16) []byte {
		return mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[at:], value)
			return b
		})
	}
	firstEntry := sfoHeaderSize
	cases := map[string][]byte{
		"empty":                 {},
		"shorter than a header": good[:sfoHeaderSize-1],
		"another magic":         mutate(func(b []byte) []byte { b[1] = 'X'; return b }),
		"truncated values":      good[:len(good)-5],
		"truncated tables":      good[:sfoHeaderSize+sfoEntrySize],
		"huge entry count":      put32(16, 0xFFFFFFFF),
		"entry count wraps":     put32(16, 0x10000000),
		"key table past values": put32(8, 0xFFFF),
		"value table past end":  put32(12, 0xFFFFFFFF),
		"key past the table":    put16(firstEntry, 0xFFFF),
		"value offset past end": put32(firstEntry+12, 0xFFFFFFF0),
		"value length past end": put32(firstEntry+4, 0xFFFFFFF0),
		"length above its max":  put32(firstEntry+8, 0),
		"number for a string":   put16(firstEntry+2, 0x0404),
		"no end of the key": mutate(func(b []byte) []byte {
			keys := binary.LittleEndian.Uint32(b[8:])
			values := binary.LittleEndian.Uint32(b[12:])
			for i := keys; i < values; i++ {
				b[i] = 'A'
			}
			return b
		}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := parseSFO(data); !errors.Is(err, ErrSFOInvalid) {
				t.Fatalf("parseSFO = %+v, %v", got, err)
			}
		})
	}
}

// Each entry points one byte further into a table with no NUL, so without the
// limit on a key every entry would copy the rest of the table. The entry limit
// keeps even that failure at a few hundred megabytes.
func TestParseSFORefusesAMegabyteOfSuffixes(t *testing.T) {
	const entries = 200
	data := make([]byte, maxSFOSize)
	copy(data, sfoMagic)
	const keys = sfoHeaderSize + entries*sfoEntrySize
	binary.LittleEndian.PutUint32(data[8:], keys)
	binary.LittleEndian.PutUint32(data[12:], maxSFOSize)
	binary.LittleEndian.PutUint32(data[16:], entries)
	for i := uint16(0); i < entries; i++ {
		binary.LittleEndian.PutUint16(data[sfoHeaderSize+int(i)*sfoEntrySize:], i)
	}
	for i := keys; i < maxSFOSize; i++ {
		data[i] = 'A'
	}
	if got, err := parseSFO(data); !errors.Is(err, ErrSFOInvalid) {
		t.Fatalf("parseSFO = %+v, %v", got, err)
	}
}

func TestParseSFOLimits(t *testing.T) {
	entries := func(n int) map[string]string {
		fields := make(map[string]string, n)
		for i := range n {
			fields[fmt.Sprintf("KEY_%03d", i)] = "x"
		}
		return fields
	}
	cases := []struct {
		name   string
		fields map[string]string
		ok     bool
	}{
		{"entries at the limit", entries(maxSFOEntries), true},
		{"entries over the limit", entries(maxSFOEntries + 1), false},
		{"a real long key", map[string]string{"SAVE_DATA_TRANSFER_TITLE_ID_LIST_1": "x"}, true},
		{"a key at the limit", map[string]string{strings.Repeat("K", maxSFOKey): "x"}, true},
		{"a key over the limit", map[string]string{strings.Repeat("K", maxSFOKey+1): "x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseSFO(sfotest.Bytes(c.fields))
			if c.ok && err != nil {
				t.Fatal(err)
			}
			if !c.ok && !errors.Is(err, ErrSFOInvalid) {
				t.Fatalf("parseSFO = %v", err)
			}
		})
	}
}

func TestPackageReadErrorTellsTheDiskFromThePackage(t *testing.T) {
	disk := &fs.PathError{Op: "read", Path: "game.vpk", Err: errors.New("device error")}
	if err := packageReadError(fmt.Errorf("read: %w", disk)); !errors.Is(err, ErrImageUnreadable) || errors.Is(err, ErrPackageInvalid) {
		t.Fatalf("disk failure = %v", err)
	}
	if err := packageReadError(zip.ErrChecksum); !errors.Is(err, ErrPackageInvalid) || errors.Is(err, ErrImageUnreadable) {
		t.Fatalf("broken package = %v", err)
	}
}

func TestParseSFORefusesARepeatedKey(t *testing.T) {
	data := gameSFO("PCSE00001", "gd")
	binary.LittleEndian.PutUint16(data[sfoHeaderSize+sfoEntrySize:], binary.LittleEndian.Uint16(data[sfoHeaderSize:]))
	if _, err := parseSFO(data); !errors.Is(err, ErrSFOInvalid) {
		t.Fatalf("parseSFO = %v", err)
	}
}

func TestReadGameInfoFromPackage(t *testing.T) {
	vita := mustPlatform(t, Vita)
	valid := gameSFO("PCSE00001", "gd")
	oversized := append(bytes.Clone(valid), make([]byte, maxSFOSize)...)
	cases := []struct {
		name    string
		entries map[string][]byte
		want    string
		err     error
	}{
		{name: "a package", entries: map[string][]byte{"sce_sys/param.sfo": valid, "eboot.bin": []byte("x")}, want: "PCSE00001"},
		{name: "backslashes", entries: map[string][]byte{`sce_sys\param.sfo`: valid}, want: "PCSE00001"},
		{name: "no param.sfo", entries: map[string][]byte{"eboot.bin": []byte("x")}, err: ErrSFOMissing},
		{name: "param.sfo in another folder", entries: map[string][]byte{"param.sfo": valid}, err: ErrSFOMissing},
		{name: "climbs out", entries: map[string][]byte{"../sce_sys/param.sfo": valid}, err: ErrPackageInvalid},
		{name: "climbs out beside the game", entries: map[string][]byte{
			"sce_sys/param.sfo": valid, "a/../../evil.dll": []byte("x"),
		}, err: ErrPackageInvalid},
		{name: "climbs out with backslashes", entries: map[string][]byte{
			"sce_sys/param.sfo": valid, `..\evil.dll`: []byte("x"),
		}, err: ErrPackageInvalid},
		{name: "absolute name", entries: map[string][]byte{"sce_sys/param.sfo": valid, "/evil.dll": []byte("x")}, err: ErrPackageInvalid},
		{name: "two copies", entries: map[string][]byte{"sce_sys/param.sfo": valid, "SCE_SYS/PARAM.SFO": valid}, err: ErrPackageInvalid},
		{name: "oversized", entries: map[string][]byte{"sce_sys/param.sfo": oversized}, err: ErrSFOInvalid},
		{name: "truncated", entries: map[string][]byte{"sce_sys/param.sfo": valid[:30]}, err: ErrSFOInvalid},
		{name: "homebrew id", entries: map[string][]byte{"sce_sys/param.sfo": gameSFO("VITASHELL", "gd")}, want: "VITASHELL"},
		{name: "lowercase id", entries: map[string][]byte{"sce_sys/param.sfo": gameSFO("pcse00001", "gd")}, err: ErrTitleIDInvalid},
		{name: "id with a path in it", entries: map[string][]byte{"sce_sys/param.sfo": gameSFO("../PCSE01", "gd")}, err: ErrTitleIDInvalid},
		{name: "id too long", entries: map[string][]byte{"sce_sys/param.sfo": gameSFO("PCSE000011", "gd")}, err: ErrTitleIDInvalid},
		{name: "no id", entries: map[string][]byte{"sce_sys/param.sfo": sfotest.Bytes(map[string]string{"CATEGORY": "gd"})}, err: ErrTitleIDInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			image := writeZip(t, filepath.Join(t.TempDir(), "game.vpk"), c.entries)
			info, err := vita.ReadGameInfo(t.Context(), image)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("ReadGameInfo = %+v, %v, want %v", info, err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.TitleID != c.want {
				t.Fatalf("TitleID = %q, want %q", info.TitleID, c.want)
			}
		})
	}
}

func TestReadGameInfoRefusesWhatIsNotAPackage(t *testing.T) {
	vita := mustPlatform(t, Vita)
	dir := t.TempDir()
	notZip := plant(t, filepath.Join(dir, "bad.vpk"), []byte("not a zip"))
	if _, err := vita.ReadGameInfo(t.Context(), notZip); !errors.Is(err, ErrPackageInvalid) {
		t.Fatalf("not a zip: %v", err)
	}
	empty := plant(t, filepath.Join(dir, "empty.vpk"), nil)
	if _, err := vita.ReadGameInfo(t.Context(), empty); !errors.Is(err, ErrPackageInvalid) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := vita.ReadGameInfo(t.Context(), filepath.Join(dir, "gone.vpk")); !errors.Is(err, ErrImageMissing) {
		t.Fatalf("missing: %v", err)
	}
}

func TestReadGameInfoFromFolder(t *testing.T) {
	root := t.TempDir()
	image := plant(t, filepath.Join(root, "eboot.bin"), []byte("x"))
	proof := filepath.Join(root, "sce_sys", "param.sfo")
	vita := mustPlatform(t, Vita)

	if _, err := vita.ReadGameInfo(t.Context(), image); !errors.Is(err, ErrImageIncomplete) {
		t.Fatalf("no proof: %v", err)
	}
	plant(t, proof, gameSFO("PCSE00001", "gd"))
	info, err := vita.ReadGameInfo(t.Context(), image)
	if err != nil || info.TitleID != "PCSE00001" {
		t.Fatalf("ReadGameInfo = %+v, %v", info, err)
	}
	plant(t, proof, make([]byte, maxSFOSize+1))
	if _, err := vita.ReadGameInfo(t.Context(), image); !errors.Is(err, ErrSFOInvalid) {
		t.Fatalf("oversized: %v", err)
	}
	plant(t, proof, gameSFO("PCSE00001", "gd")[:40])
	if _, err := vita.ReadGameInfo(t.Context(), image); !errors.Is(err, ErrSFOInvalid) {
		t.Fatalf("truncated: %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := vita.ReadGameInfo(cancelled, image); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// A title on the PS3 can be homebrew with a free-form id: only a console that
// starts games by id demands the strict form.
func TestReadGameInfoKeepsAFreeFormIdElsewhere(t *testing.T) {
	root := t.TempDir()
	image := plant(t, filepath.Join(root, "USRDIR", "EBOOT.BIN"), []byte("x"))
	plant(t, filepath.Join(root, "PARAM.SFO"), gameSFO("RPCS30001", "HG"))
	info, err := mustPlatform(t, PS3).ReadGameInfo(t.Context(), image)
	if err != nil || info.TitleID != "RPCS30001" {
		t.Fatalf("ReadGameInfo = %+v, %v", info, err)
	}
}

func TestIsUpdateKeepsTheCaseOfEachConsole(t *testing.T) {
	cases := []struct {
		platform, category string
		want               bool
	}{
		{PS3, "GD", true},
		{PS3, "gd", false},
		{PS3, "DG", false},
		{PS3, "HG", false},
		{PS4, "gd", false},
		{PS4, "gp", true},
		{Vita, "gd", false},
		{Vita, "gp", true},
		{PS2, "GD", false},
	}
	for _, c := range cases {
		if got := mustPlatform(t, c.platform).IsUpdate(GameInfo{Category: c.category}); got != c.want {
			t.Errorf("%s %s: IsUpdate = %v", c.platform, c.category, got)
		}
	}
}

func TestStorageFolders(t *testing.T) {
	vita := mustPlatform(t, Vita)
	cases := []struct {
		path   string
		stored string
		data   bool
	}{
		{`E:\Vita3K\ux0\app\PCSE00001\eboot.bin`, "PCSE00001", false},
		{`E:\Vita3K\Ux0\App\PCSE00001\eboot.bin`, "PCSE00001", false},
		{"/home/a/ux0/app/PCSE00001/game.vpk", "PCSE00001", false},
		{`E:\Vita3K\ux0\app\game.vpk`, "", false},
		{`E:\ux0.\app\PCSE00001\eboot.bin`, "PCSE00001", false},
		{`E:\ux0 \app.\PCSE00001.\eboot.bin`, "PCSE00001", false},
		{`E:\ux0.\patch\PCSE00001\eboot.bin`, "", true},
		{`E:\ux0x\app\PCSE00001\eboot.bin`, "", false},
		{`E:\xux0\app\PCSE00001\eboot.bin`, "", false},
		{`E:\ux0\apps\PCSE00001\eboot.bin`, "", false},
		{`E:\Vita3K\ux0\patch\PCSE00001\eboot.bin`, "", true},
		{`E:\Vita3K\UX0\ADDCONT\PCSE00001\a\eboot.bin`, "", true},
		{`E:\Games\eboot.bin`, "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := vita.StoredTitle(c.path); got != c.stored {
			t.Errorf("%q: StoredTitle = %q, want %q", c.path, got, c.stored)
		}
		if got := vita.InEmulatorData(c.path); got != c.data {
			t.Errorf("%q: InEmulatorData = %v, want %v", c.path, got, c.data)
		}
	}
	ps4 := mustPlatform(t, PS4)
	if ps4.StoredTitle(`E:\ux0\app\CUSA00001\eboot.bin`) != "" || ps4.InEmulatorData(`E:\ux0\patch\CUSA00001\eboot.bin`) {
		t.Fatal("a console whose emulator installs nothing has no such storage")
	}
}

func TestIsAddon(t *testing.T) {
	cases := []struct {
		platform, category string
		want               bool
	}{
		{Vita, "ac", true},
		{PS4, "ac", true},
		{Vita, "gd", false},
		{PS3, "ac", false},
	}
	for _, c := range cases {
		if got := mustPlatform(t, c.platform).IsAddon(GameInfo{Category: c.category}); got != c.want {
			t.Errorf("%s %s: IsAddon = %v", c.platform, c.category, got)
		}
	}
}

func TestCommandOfNextGenConsoles(t *testing.T) {
	folder := filepath.Join("Games", "PCSE00001")
	cases := []struct {
		platform string
		image    string
		want     []string
	}{
		{Vita, filepath.Join("Games", "game.vpk"), []string{"--frontend", "sdl", "-F", filepath.Join("Games", "game.vpk")}},
		{Vita, filepath.Join(folder, "eboot.bin"), []string{"--frontend", "sdl", "-F", folder}},
		{PS4, filepath.Join("G", "eboot.bin"), []string{"-f", "true", "-g", filepath.Join("G", "eboot.bin")}},
		{PS3, filepath.Join("G", "USRDIR", "EBOOT.BIN"), []string{"--no-gui", "--fullscreen", filepath.Join("G", "USRDIR", "EBOOT.BIN")}},
	}
	for _, c := range cases {
		if got := mustPlatform(t, c.platform).Command(c.image); !slices.Equal(got, c.want) {
			t.Errorf("%s: %q, want %q", c.platform, got, c.want)
		}
	}
}

func TestInstalledCommand(t *testing.T) {
	vita := mustPlatform(t, Vita)
	got, err := vita.InstalledCommand("PCSE00001")
	if err != nil || !slices.Equal(got, []string{"--frontend", "sdl", "-F", "-r", "PCSE00001"}) {
		t.Fatalf("InstalledCommand = %q, %v", got, err)
	}
	for _, bad := range []string{"", "pcse00001", "PCSE0000", "PCSE000011", "--fullscreen", "PCSE0000\n"} {
		if got, err := vita.InstalledCommand(bad); !errors.Is(err, ErrTitleIDInvalid) {
			t.Errorf("%q: %q, %v", bad, got, err)
		}
	}
	if got, err := vita.InstalledCommand("VITASHELL"); err != nil || got[4] != "VITASHELL" {
		t.Fatalf("a homebrew id: %q, %v", got, err)
	}
	if !vita.InstallGone(4) || vita.InstallGone(0) || vita.InstallGone(1) || vita.InstallGone(-1) {
		t.Fatal("only the emulator's answer about an unknown title counts")
	}
	ps4 := mustPlatform(t, PS4)
	if ps4.RemembersInstall() || ps4.InstallGone(0) || ps4.InstallGone(4) {
		t.Fatal("the PS4 emulator is given the game every time")
	}
	if _, err := ps4.InstalledCommand("CUSA00001"); err == nil {
		t.Fatal("a command for a console that keeps nothing")
	}
}

func TestImagePatternsNameTheStartingFiles(t *testing.T) {
	cases := map[string]string{PS3: "EBOOT.BIN", PS4: "eboot.bin", Vita: "*.vpk;eboot.bin"}
	for id, want := range cases {
		if got, err := ImagePatterns([]string{id}); err != nil || got != want {
			t.Errorf("%s = %q, %v, want %q", id, got, err, want)
		}
	}
	all, err := ImagePatterns(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.ToLower(all), "eboot.bin"); n != 1 {
		t.Fatalf("all = %q: name repeated %d times", all, n)
	}
}

func TestListShowsARowForEveryConsoleWithoutNulls(t *testing.T) {
	for _, p := range Platforms() {
		row := (&Service{}).statusOf(p, "")
		if row.Extensions == nil || row.ImageNames == nil {
			t.Errorf("%s: a nil list reaches the interface as null: %+v", p.ID, row)
		}
	}
}
