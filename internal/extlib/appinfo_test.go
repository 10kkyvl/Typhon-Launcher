package extlib

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"typhon/internal/library"
)

type kvNode struct {
	name     string
	str      string
	children []kvNode
	nested   bool
}

func kvObj(name string, children ...kvNode) kvNode {
	return kvNode{name: name, nested: true, children: children}
}

func kvStr(name, val string) kvNode { return kvNode{name: name, str: val} }

type appInfoBuilder struct {
	t       *testing.T
	magic   uint32
	strs    []string
	entries bytes.Buffer
}

func (b *appInfoBuilder) key(buf *bytes.Buffer, name string) {
	if b.magic != appInfoMagic29 {
		buf.WriteString(name)
		buf.WriteByte(0)
		return
	}
	idx := -1
	for i, s := range b.strs {
		if s == name {
			idx = i
		}
	}
	if idx < 0 {
		b.strs = append(b.strs, name)
		idx = len(b.strs) - 1
	}
	putU32(buf, u32(b.t, idx))
}

func u32(t *testing.T, n int) uint32 {
	t.Helper()
	v, err := strconv.ParseUint(strconv.Itoa(n), 10, 32)
	if err != nil {
		t.Fatalf("%d does not fit uint32: %v", n, err)
	}
	return uint32(v)
}

func putU32(buf *bytes.Buffer, v uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], v)
	buf.Write(raw[:])
}

func (b *appInfoBuilder) node(buf *bytes.Buffer, n kvNode) {
	if n.nested {
		buf.WriteByte(kvNested)
		b.key(buf, n.name)
		for _, c := range n.children {
			b.node(buf, c)
		}
		buf.WriteByte(kvEnd)
		return
	}
	buf.WriteByte(kvString)
	b.key(buf, n.name)
	buf.WriteString(n.str)
	buf.WriteByte(0)
}

func (b *appInfoBuilder) add(appID uint32, typ string) {
	var kv bytes.Buffer
	common := []kvNode{kvStr("name", "x")}
	if typ != "" {
		common = append(common, kvStr("type", typ))
	}
	b.node(&kv, kvObj("appinfo", kvObj("common", common...), kvObj("config", kvStr("installdir", "d"))))
	kv.WriteByte(kvEnd)
	meta := 60
	if b.magic == appInfoMagic27 {
		meta = 40
	}
	putU32(&b.entries, appID)
	putU32(&b.entries, u32(b.t, meta+kv.Len()))
	b.entries.Write(make([]byte, meta))
	b.entries.Write(kv.Bytes())
}

func (b *appInfoBuilder) bytes() []byte {
	var out bytes.Buffer
	putU32(&out, b.magic)
	putU32(&out, 1)
	body := append(append([]byte(nil), b.entries.Bytes()...), 0, 0, 0, 0)
	if b.magic != appInfoMagic29 {
		out.Write(body)
		return out.Bytes()
	}
	var off [8]byte
	binary.LittleEndian.PutUint64(off[:], uint64(16+len(body)))
	out.Write(off[:])
	out.Write(body)
	putU32(&out, u32(b.t, len(b.strs)))
	for _, s := range b.strs {
		out.WriteString(s)
		out.WriteByte(0)
	}
	return out.Bytes()
}

func buildAppInfo(t *testing.T, magic uint32, apps map[uint32]string) []byte {
	t.Helper()
	b := &appInfoBuilder{t: t, magic: magic}
	for _, id := range []uint32{1, 2, 3, 4} {
		if typ, ok := apps[id]; ok {
			b.add(id, typ)
		}
	}
	return b.bytes()
}

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadAppTypes(t *testing.T) {
	apps := map[uint32]string{1: "Game", 2: "Tool", 3: "Application", 4: ""}
	wanted := map[uint32]bool{1: true, 2: true, 3: true, 4: true, 99: true}
	for _, magic := range []uint32{appInfoMagic27, appInfoMagic28, appInfoMagic29} {
		t.Run(strconv.FormatUint(uint64(magic), 16), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "appinfo.vdf")
			writeBytes(t, path, buildAppInfo(t, magic, apps))
			got, err := readAppTypes(context.Background(), path, wanted)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"1": "game", "2": "tool", "3": "application"}
			if len(got) != len(want) {
				t.Fatalf("types = %v, want %v", got, want)
			}
			for k, v := range want {
				if got[k] != v {
					t.Fatalf("types[%s] = %q, want %q (all: %v)", k, got[k], v, got)
				}
			}
		})
	}
}

func TestReadAppTypesSkipsUnwanted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appinfo.vdf")
	writeBytes(t, path, buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game", 2: "Tool"}))
	got, err := readAppTypes(context.Background(), path, map[uint32]bool{2: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["2"] != "tool" {
		t.Fatalf("types = %v, want only app 2", got)
	}
}

func TestReadAppTypesErrors(t *testing.T) {
	good := buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game", 2: "Tool"})
	badMagic := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(badMagic, 0x07564430)
	badTable := append([]byte(nil), good...)
	binary.LittleEndian.PutUint64(badTable[8:], uint64(len(good)+100))
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{"empty file", nil, errAppInfoCorrupt},
		{"short header", good[:6], errAppInfoCorrupt},
		{"unknown magic", badMagic, errAppInfoMagic},
		{"table offset past end", badTable, errAppInfoNoStrings},
		{"truncated v28", buildAppInfo(t, appInfoMagic28, map[uint32]string{1: "Game"})[:50], errAppInfoCorrupt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "appinfo.vdf")
			writeBytes(t, path, tc.data)
			_, err := readAppTypes(context.Background(), path, map[uint32]bool{1: true, 2: true})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestReadAppTypesEveryPrefixFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appinfo.vdf")
	for _, magic := range []uint32{appInfoMagic28, appInfoMagic29} {
		good := buildAppInfo(t, magic, map[uint32]string{1: "Game", 2: "Tool"})
		for n := 0; n < len(good); n++ {
			writeBytes(t, path, good[:n])
			if _, err := readAppTypes(context.Background(), path, map[uint32]bool{1: true, 2: true}); err == nil {
				t.Fatalf("magic %x: prefix of %d bytes parsed without an error", magic, n)
			}
		}
	}
}

func TestReadAppTypesMissingAndCancelled(t *testing.T) {
	dir := t.TempDir()
	if _, err := readAppTypes(context.Background(), filepath.Join(dir, "none.vdf"), nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want ErrNotExist", err)
	}
	path := filepath.Join(dir, "appinfo.vdf")
	writeBytes(t, path, buildAppInfo(t, appInfoMagic28, map[uint32]string{1: "Game"}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readAppTypes(ctx, path, map[uint32]bool{1: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

type sizeInfo struct {
	os.FileInfo
	size int64
}

func (s sizeInfo) Size() int64 { return s.size }

type fakeInfoFile struct {
	*bytes.Reader
	size int64
}

func (f fakeInfoFile) Stat() (os.FileInfo, error) { return sizeInfo{size: f.size}, nil }

func TestParseAppInfoRejectsHugeFile(t *testing.T) {
	f := fakeInfoFile{Reader: bytes.NewReader(nil), size: maxAppInfoBytes + 1}
	if _, err := parseAppInfo(context.Background(), f, nil); !errors.Is(err, errAppInfoTooLarge) {
		t.Fatalf("error = %v, want errAppInfoTooLarge", err)
	}
}

func TestParseAppInfoRejectsHugeEntry(t *testing.T) {
	var out bytes.Buffer
	putU32(&out, appInfoMagic28)
	putU32(&out, 1)
	putU32(&out, 1)
	putU32(&out, maxAppInfoEntryBytes+1000)
	f := fakeInfoFile{Reader: bytes.NewReader(out.Bytes()), size: int64(out.Len())}
	if _, err := parseAppInfo(context.Background(), f, map[uint32]bool{1: true}); !errors.Is(err, errAppInfoTooLarge) {
		t.Fatalf("error = %v, want errAppInfoTooLarge", err)
	}
}

func steamScanFixture(t *testing.T, info []byte) string {
	t.Helper()
	root := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	for _, d := range []string{"Wallpaper", "Real"} {
		if err := os.MkdirAll(filepath.Join(apps, "common", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(apps, "appmanifest_1.acf"), acf("1", "Real Game", "Real", "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_2.acf"), acf("2", "Wallpaper Engine", "Wallpaper", "4"))
	if info != nil {
		writeBytes(t, filepath.Join(root, "appcache", "appinfo.vdf"), info)
	}
	return root
}

func TestScanSteamDropsNonGames(t *testing.T) {
	full := buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game", 2: "Tool"})
	tests := []struct {
		name      string
		info      []byte
		wantGames []string
		wantNot   []string
		wantErr   error
	}{
		{"tool is not a game", full, []string{"1"}, []string{"2"}, nil},
		{"application is not a game", buildAppInfo(t, appInfoMagic28, map[uint32]string{1: "game", 2: "Application"}), []string{"1"}, []string{"2"}, nil},
		{"demo stays a game", buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game", 2: "Demo"}), []string{"1", "2"}, nil, nil},
		{"app missing from appinfo stays a game", buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game"}), []string{"1", "2"}, nil, nil},
		{"no appinfo file: type unknown", nil, []string{"1", "2"}, nil, nil},
		{"truncated appinfo fails the scan", full[:30], nil, nil, errAppInfoNoStrings},
		{"unknown magic fails the scan", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, nil, nil, errAppInfoMagic},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := steamScanFixture(t, tc.info)
			got := scanSteam(context.Background(), root)
			if tc.wantErr != nil {
				if !errors.Is(got.Err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", got.Err, tc.wantErr)
				}
				if len(got.Games) != 0 {
					t.Fatalf("games on failed scan: %+v", got.Games)
				}
				return
			}
			if got.Err != nil {
				t.Fatal(got.Err)
			}
			ids := map[string]bool{}
			for _, g := range got.Games {
				ids[g.AppID] = true
			}
			if len(ids) != len(tc.wantGames) {
				t.Fatalf("games = %v, want %v", ids, tc.wantGames)
			}
			for _, id := range tc.wantGames {
				if !ids[id] {
					t.Fatalf("games = %v, want %v", ids, tc.wantGames)
				}
			}
			if len(got.NotGames) != len(tc.wantNot) {
				t.Fatalf("not games = %v, want %v", got.NotGames, tc.wantNot)
			}
		})
	}
}

func TestSyncOnceRemovesNonGames(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1"), steamRef("2")}}
	scan := ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}, NotGames: []string{"2", "3"}}
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{scan}, nil
	})
	if err := s.syncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(lib.removed) != 1 || lib.removed[0] != steamRef("2") {
		t.Fatalf("removed = %v, want only the imported non-game 2", lib.removed)
	}
	if !lib.syncPresent[library.ProviderSteam]["2"] {
		t.Fatalf("non-game must not be marked uninstalled: present = %v", lib.syncPresent)
	}
}

func TestSyncOnceFailedScanChangesNothing(t *testing.T) {
	lib := &fakeLibrary{refs: []library.ExternalRef{steamRef("1"), steamRef("2")}}
	scan := ProviderScan{Provider: library.ProviderSteam, Err: errAppInfoCorrupt}
	s := syncService(t, lib, &fakeCatalog{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{scan}, nil
	})
	err := s.syncOnce(context.Background())
	if !errors.Is(err, errAppInfoCorrupt) {
		t.Fatalf("error = %v, want errAppInfoCorrupt", err)
	}
	if len(lib.removed) != 0 || lib.syncPresent[library.ProviderSteam] != nil {
		t.Fatalf("failed scan touched the library: removed=%v present=%v", lib.removed, lib.syncPresent)
	}
}

func TestImportRemovesImportedNonGames(t *testing.T) {
	lib := &fakeLibrary{}
	scan := ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}, NotGames: []string{"2"}}
	s := started(t, lib, &fakeCatalog{}, &fakeMetadata{}, func(context.Context) ([]ProviderScan, error) {
		return []ProviderScan{scan}, nil
	})
	if _, err := s.Import(); err != nil {
		t.Fatal(err)
	}
	if len(lib.removed) != 1 || lib.removed[0] != steamRef("2") {
		t.Fatalf("removed = %v, want the non-game 2", lib.removed)
	}
}

func TestImportRemovalFailureDoesNotStopImport(t *testing.T) {
	tests := []struct {
		name      string
		removeErr error
	}{
		{"removal ok", nil},
		{"removal fails", errors.New("disk full")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lib := &fakeLibrary{removeErr: tc.removeErr}
			scan := ProviderScan{Provider: library.ProviderSteam, Games: []Candidate{steamCandidate("1", "A")}, NotGames: []string{"2"}}
			s := started(t, lib, &fakeCatalog{}, &fakeMetadata{}, func(context.Context) ([]ProviderScan, error) {
				return []ProviderScan{scan}, nil
			})
			sum, err := s.Import()
			if len(lib.applied) != 1 || sum.Added != 1 {
				t.Fatalf("import did not apply the games: applied=%v summary=%+v", lib.applied, sum)
			}
			if !errors.Is(err, tc.removeErr) {
				t.Fatalf("error = %v, want %v", err, tc.removeErr)
			}
		})
	}
}

func TestAppTypeCacheRereadsOnlyWhenNeeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appinfo.vdf")
	writeBytes(t, path, buildAppInfo(t, appInfoMagic29, map[uint32]string{1: "Game", 2: "Tool", 3: "Game"}))
	reads := 0
	cache := &appTypeCache{read: func(ctx context.Context, p string, wanted map[uint32]bool) (map[string]string, error) {
		reads++
		return readAppTypes(ctx, p, wanted)
	}}
	ctx := context.Background()
	get := func(ids ...uint32) map[string]string {
		t.Helper()
		wanted := map[uint32]bool{}
		for _, id := range ids {
			wanted[id] = true
		}
		got, err := cache.get(ctx, path, wanted)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := get(1, 2); got["2"] != "tool" || reads != 1 {
		t.Fatalf("first read: types=%v reads=%d", got, reads)
	}
	get(1, 2)
	get(2)
	if reads != 1 {
		t.Fatalf("unchanged file was read again: reads=%d", reads)
	}
	if got := get(3); got["3"] != "game" || got["2"] != "tool" || reads != 2 {
		t.Fatalf("new appid: types=%v reads=%d", got, reads)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	get(1, 2)
	if reads != 3 {
		t.Fatalf("changed mtime was not re-read: reads=%d", reads)
	}
	if _, err := cache.get(ctx, filepath.Join(filepath.Dir(path), "none.vdf"), nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v, want ErrNotExist", err)
	}
}

func TestReadAppTypesAlternateEnd(t *testing.T) {
	var kv bytes.Buffer
	kv.WriteByte(kvNested)
	kv.WriteString("appinfo\x00")
	kv.WriteByte(kvNested)
	kv.WriteString("common\x00")
	kv.WriteByte(kvString)
	kv.WriteString("type\x00Tool\x00")
	kv.WriteByte(kvEndAlt)
	kv.WriteByte(kvEndAlt)
	kv.WriteByte(kvEndAlt)
	var out bytes.Buffer
	putU32(&out, appInfoMagic28)
	putU32(&out, 1)
	putU32(&out, 5)
	putU32(&out, u32(t, 60+kv.Len()))
	out.Write(make([]byte, 60))
	out.Write(kv.Bytes())
	putU32(&out, 0)
	path := filepath.Join(t.TempDir(), "appinfo.vdf")
	writeBytes(t, path, out.Bytes())
	got, err := readAppTypes(context.Background(), path, map[uint32]bool{5: true})
	if err != nil || got["5"] != "tool" {
		t.Fatalf("types = %v, err = %v", got, err)
	}
}
