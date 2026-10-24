package wrappers

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"typhon/internal/library"
)

const (
	machineX86 = 0x14c
	machineX64 = 0x8664
)

func u32(n int) uint32 { return uint32(n & 0xffffffff) }

func buildPE(machine uint16, dlls []string, delay ...string) []byte {
	const rawOff, va = 0x200, 0x1000
	plus := machine == machineX64
	optSize := 224
	if plus {
		optSize = 240
	}
	descLen := (len(dlls) + 1) * 20
	delayLen := (len(delay) + 1) * 32
	data := make([]byte, descLen+delayLen)
	for i, d := range delay {
		nameRVA := u32(va + len(data))
		data = append(data, []byte(d+"\x00")...)
		desc := data[descLen+i*32:]
		binary.LittleEndian.PutUint32(desc[0:], 1)
		binary.LittleEndian.PutUint32(desc[4:], nameRVA)
	}
	for i, d := range dlls {
		nameRVA := u32(va + len(data))
		data = append(data, []byte(d+"\x00")...)
		desc := data[i*20:]
		binary.LittleEndian.PutUint32(desc[0:], va)
		binary.LittleEndian.PutUint32(desc[12:], nameRVA)
		binary.LittleEndian.PutUint32(desc[16:], va)
	}
	hdr := make([]byte, rawOff)
	copy(hdr, "MZ")
	binary.LittleEndian.PutUint32(hdr[0x3c:], 64)
	copy(hdr[64:], "PE\x00\x00")
	coff := hdr[68:]
	binary.LittleEndian.PutUint16(coff[0:], machine)
	binary.LittleEndian.PutUint16(coff[2:], 1)
	binary.LittleEndian.PutUint16(coff[16:], uint16(optSize))
	binary.LittleEndian.PutUint16(coff[18:], 0x2)
	opt := hdr[88:]
	var dd []byte
	if plus {
		binary.LittleEndian.PutUint16(opt[0:], 0x20b)
		binary.LittleEndian.PutUint32(opt[108:], 16)
		dd = opt[112:]
	} else {
		binary.LittleEndian.PutUint16(opt[0:], 0x10b)
		binary.LittleEndian.PutUint32(opt[92:], 16)
		dd = opt[96:]
	}
	binary.LittleEndian.PutUint32(dd[8:], va)
	binary.LittleEndian.PutUint32(dd[12:], u32(descLen))
	if len(delay) > 0 {
		binary.LittleEndian.PutUint32(dd[13*8:], u32(va+descLen))
		binary.LittleEndian.PutUint32(dd[13*8+4:], u32(delayLen))
	}
	sec := hdr[88+optSize:]
	copy(sec, ".idata")
	binary.LittleEndian.PutUint32(sec[8:], u32(len(data)))
	binary.LittleEndian.PutUint32(sec[12:], va)
	binary.LittleEndian.PutUint32(sec[16:], u32(len(data)))
	binary.LittleEndian.PutUint32(sec[20:], rawOff)
	binary.LittleEndian.PutUint32(sec[36:], 0x40000040)
	return append(hdr, data...)
}

type archEntry struct {
	name string
	data string
	kind byte
}

func makeTgz(t *testing.T, entries []archEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: kind, Mode: 0o644, Size: int64(len(e.data))}
		if kind == tar.TypeSymlink {
			h.Linkname, h.Size = e.data, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(e.data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, entries []archEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var dxvkDlls = []string{"d3d9", "d3d10core", "d3d11", "dxgi"}

func dxvkEntries() []archEntry {
	var out []archEntry
	for _, n := range dxvkDlls {
		out = append(out, archEntry{name: "dxvk-1/x32/" + n + ".dll", data: "DXVK-" + n})
	}
	return out
}

func dxvkRelease(url string, archive []byte) release {
	var files []archiveFile
	for _, n := range dxvkDlls {
		files = append(files, archiveFile{From: "dxvk-1/x32/" + n + ".dll", To: "x86/" + n + ".dll"})
	}
	return release{
		Wrapper: WrapperDXVK, Version: "1", URL: url, SHA256: hashBytes(archive),
		Kind: "tgz", Files: files, Presets: []string{"default"},
	}
}

const sampleConf = "[General]\r\nFullScreenMode = true\r\n\r\n[Glide]\r\nResolution                          = unforced\r\n3DfxWatermark                       = true\r\n\r\n[DirectX]\r\nResolution                          = unforced\r\ndgVoodooWatermark                   = true\r\n"

func dgvoodooRelease(url string, archive []byte) release {
	return release{
		Wrapper: WrapperDgVoodoo, Version: "1", URL: url, SHA256: hashBytes(archive), Kind: "zip",
		Files: []archiveFile{
			{From: "dgVoodoo.conf", To: confName},
			{From: "MS/x86/D3D9.dll", To: "x86/d3d9.dll"},
			{From: "MS/x86/DDraw.dll", To: "x86/ddraw.dll"},
			{From: "MS/x86/D3DImm.dll", To: "x86/d3dimm.dll"},
		},
		Presets: []string{"default", "1080p", "native"},
	}
}

func dgvoodooEntries() []archEntry {
	return []archEntry{
		{name: "dgVoodoo.conf", data: sampleConf},
		{name: "MS/x86/D3D9.dll", data: "DGV-d3d9"},
		{name: "MS/x86/DDraw.dll", data: "DGV-ddraw"},
		{name: "MS/x86/D3DImm.dll", data: "DGV-d3dimm"},
	}
}

type fakeGames struct {
	mu      sync.Mutex
	game    library.Game
	running bool
}

func (f *fakeGames) Find(id string) (library.Game, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.game.ID {
		return library.Game{}, os.ErrNotExist
	}
	return f.game, nil
}

func (f *fakeGames) IsRunning(string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *fakeGames) setRunning(v bool) {
	f.mu.Lock()
	f.running = v
	f.mu.Unlock()
}

type fixture struct {
	svc    *Service
	games  *fakeGames
	cfg    string
	dir    string
	hits   *atomic.Int32
	events *eventLog
	srv    *httptest.Server
}

type eventLog struct {
	mu     sync.Mutex
	stages []string
}

func (e *eventLog) add(stage string) {
	e.mu.Lock()
	e.stages = append(e.stages, stage)
	e.mu.Unlock()
}

func (e *eventLog) has(stage string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.stages {
		if s == stage {
			return true
		}
	}
	return false
}

func allowHost(host string) func(*url.URL) error {
	return func(u *url.URL) error {
		if u.Host != host {
			return ErrForeignHost
		}
		return nil
	}
}

func newFixture(t *testing.T, archive []byte, mk func(url string, archive []byte) release, machine uint16, imports []string) *fixture {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := w.Write(archive); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	rel := mk(srv.URL+"/a", archive)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, buildPE(machine, imports), 0o600); err != nil {
		t.Fatal(err)
	}
	games := &fakeGames{game: library.Game{ID: "g1", InstallDir: dir, Executable: exe}}
	cfg := t.TempDir()
	svc, err := newService(cfg, games, map[string]release{rel.Wrapper: rel}, newDownloader(allowHost(u.Host)), true)
	if err != nil {
		t.Fatal(err)
	}
	ev := &eventLog{}
	svc.emit = func(_ string, data any) {
		if p, ok := data.(Progress); ok {
			ev.add(p.Stage)
		}
	}
	return &fixture{svc: svc, games: games, cfg: cfg, dir: dir, hits: hits, events: ev, srv: srv}
}

func (f *fixture) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) write(t *testing.T, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) exists(name string) bool {
	_, err := os.Lstat(filepath.Join(f.dir, name))
	return err == nil
}
