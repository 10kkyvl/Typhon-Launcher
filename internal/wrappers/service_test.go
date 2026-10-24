package wrappers

import (
	"archive/tar"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/storage"
)

func TestSuggest(t *testing.T) {
	cases := []struct {
		name    string
		imports []string
		want    string
	}{
		{"ddraw", []string{"ddraw"}, WrapperDgVoodoo},
		{"d3d8", []string{"d3d8"}, WrapperDgVoodoo},
		{"glide2x", []string{"glide2x"}, WrapperDgVoodoo},
		{"d3d9", []string{"d3d9"}, WrapperDXVK},
		{"d3d10_1", []string{"d3d10_1"}, WrapperDXVK},
		{"d3d11", []string{"d3d11"}, WrapperDXVK},
		{"ddraw wins over d3d9", []string{"d3d9", "ddraw"}, WrapperDgVoodoo},
		{"opengl only", []string{"opengl32"}, ""},
		{"nothing", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := suggest(c.imports); got != c.want {
				t.Fatalf("suggest(%v) = %q, want %q", c.imports, got, c.want)
			}
		})
	}
}

func TestParseImports(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		machine uint16
		dlls    []string
		delay   []string
		arch    string
		want    []string
	}{
		{"x86", machineX86, []string{"KERNEL32.dll", "DDRAW.dll", "d3d9.dll", "DDRAW.dll"}, nil, archX86, []string{"d3d9", "ddraw"}},
		{"x64", machineX64, []string{"KERNEL32.dll", "d3d11.dll", "OPENGL32.dll"}, nil, archX64, []string{"d3d11", "opengl32"}},
		{"no graphics", machineX86, []string{"KERNEL32.dll"}, nil, archX86, nil},
		{"single import", machineX86, []string{"d3d9.dll"}, nil, archX86, []string{"d3d9"}},
		{"delay load", machineX64, []string{"KERNEL32.dll"}, []string{"D3D11.dll"}, archX64, []string{"d3d11"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, c.name+".exe")
			if err := os.WriteFile(p, buildPE(c.machine, c.dlls, c.delay...), 0o600); err != nil {
				t.Fatal(err)
			}
			arch, got, err := parseImports(p)
			if err != nil {
				t.Fatal(err)
			}
			if arch != c.arch || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %s %v, want %s %v", arch, got, c.arch, c.want)
			}
		})
	}
	t.Run("not a PE", func(t *testing.T) {
		p := filepath.Join(dir, "x.bat")
		if err := os.WriteFile(p, []byte("@echo off\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := parseImports(p); !errors.Is(err, ErrNotPE) {
			t.Fatalf("err = %v, want ErrNotPE", err)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		_, _, err := parseImports(filepath.Join(dir, "nope.exe"))
		if err == nil || errors.Is(err, ErrNotPE) {
			t.Fatalf("err = %v, want an open error", err)
		}
	})
}

func TestTargets(t *testing.T) {
	names := func(tg []target) []string {
		var out []string
		for _, x := range tg {
			out = append(out, x.Name)
		}
		return out
	}
	cases := []struct {
		name    string
		wrapper string
		arch    string
		imports []string
		want    []string
	}{
		{"dgv ddraw", WrapperDgVoodoo, archX86, []string{"ddraw"}, []string{"d3dimm.dll", "ddraw.dll", confName}},
		{"dgv glide", WrapperDgVoodoo, archX86, []string{"glide3x"}, []string{confName, "glide.dll", "glide2x.dll", "glide3x.dll"}},
		{"dgv x64 ddraw has nothing", WrapperDgVoodoo, archX64, []string{"ddraw"}, nil},
		{"dgv x64 d3d9", WrapperDgVoodoo, archX64, []string{"d3d9"}, []string{"d3d9.dll", confName}},
		{"dxvk d3d9", WrapperDXVK, archX86, []string{"d3d9"}, []string{"d3d9.dll"}},
		{"dxvk d3d10", WrapperDXVK, archX64, []string{"d3d10_1"}, []string{"d3d10core.dll", "d3d11.dll", "dxgi.dll"}},
		{"dxvk ddraw has nothing", WrapperDXVK, archX86, []string{"ddraw"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := names(targets(c.wrapper, c.arch, c.imports))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestPatchConf(t *testing.T) {
	cases := []struct {
		preset, res string
	}{{"default", "unforced"}, {"1080p", "max_fhd"}, {"native", "desktop"}}
	for _, c := range cases {
		t.Run(c.preset, func(t *testing.T) {
			out, err := patchConf([]byte(sampleConf), c.preset)
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			for _, want := range []string{"dgVoodooWatermark                   = false\r\n", "3DfxWatermark                       = false\r\n"} {
				if !strings.Contains(s, want) {
					t.Fatalf("missing %q in %q", want, s)
				}
			}
			if strings.Count(s, "= "+c.res+"\r\n") != 2 {
				t.Fatalf("resolution not applied to both sections: %q", s)
			}
			if !strings.Contains(s, "FullScreenMode = true\r\n") {
				t.Fatalf("unrelated key changed: %q", s)
			}
		})
	}
	if _, err := patchConf([]byte("[General]\r\n"), "default"); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("missing key err = %v", err)
	}
	if _, err := patchConf([]byte(sampleConf), "bogus"); !errors.Is(err, ErrBadPreset) {
		t.Fatalf("bad preset err = %v", err)
	}
}

func TestEnableDisable(t *testing.T) {
	cases := []struct {
		name string
		orig map[string]string
	}{
		{"no originals", nil},
		{"with original d3d9", map[string]string{"d3d9.dll": "ORIGINAL-d3d9"}},
		{"with two originals", map[string]string{"d3d9.dll": "A", "dxgi.dll": "B"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ar := makeTgz(t, dxvkEntries())
			f := newFixture(t, ar, dxvkRelease, machineX86, []string{"KERNEL32.dll", "d3d11.dll"})
			for n, d := range c.orig {
				f.write(t, n, d)
			}
			ctx := context.Background()
			if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
				t.Fatal(err)
			}
			if got := f.read(t, "d3d11.dll"); got != "DXVK-d3d11" {
				t.Fatalf("d3d11.dll = %q", got)
			}
			for _, n := range []string{"d3d10core.dll", "dxgi.dll"} {
				if f.read(t, n) != "DXVK-"+strings.TrimSuffix(n, ".dll") {
					t.Fatalf("%s not placed", n)
				}
			}
			if f.exists("d3d9.dll") != (c.orig["d3d9.dll"] != "") {
				t.Fatal("d3d9.dll must stay absent unless it was there")
			}
			info, err := f.svc.Inspect(ctx, "g1")
			if err != nil {
				t.Fatal(err)
			}
			if !info.Supported || info.Arch != archX86 || info.Suggested != WrapperDXVK || info.Active == nil {
				t.Fatalf("info = %+v", info)
			}
			if info.Active.Wrapper != WrapperDXVK || info.Active.Version != "1" || info.Active.Preset != "default" ||
				!reflect.DeepEqual(info.Active.Files, []string{"d3d10core.dll", "d3d11.dll", "dxgi.dll"}) || info.Active.AppliedAt.IsZero() {
				t.Fatalf("active = %+v", info.Active)
			}
			if !f.events.has("download") || !f.events.has("apply") {
				t.Fatalf("progress stages = %v", f.events.stages)
			}
			for n, d := range c.orig {
				b, err := os.ReadFile(filepath.Join(f.cfg, "wrappers", "backup", "g1", n))
				if n == "d3d9.dll" {
					continue
				}
				if err != nil || string(b) != d {
					t.Fatalf("backup of %s = %q, %v", n, b, err)
				}
			}
			if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, ErrActive) {
				t.Fatalf("second Enable err = %v", err)
			}
			if err := f.svc.Disable(ctx, "g1"); err != nil {
				t.Fatal(err)
			}
			for _, n := range []string{"d3d9.dll", "d3d10core.dll", "d3d11.dll", "dxgi.dll"} {
				if orig, ok := c.orig[n]; ok {
					if f.read(t, n) != orig {
						t.Fatalf("%s not restored", n)
					}
				} else if f.exists(n) {
					t.Fatalf("%s must be removed", n)
				}
			}
			if _, err := os.Stat(filepath.Join(f.cfg, "wrappers", "backup", "g1")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("backup dir remains: %v", err)
			}
			info, err = f.svc.Inspect(ctx, "g1")
			if err != nil || info.Active != nil {
				t.Fatalf("after Disable: %+v, %v", info, err)
			}
			if err := f.svc.Disable(ctx, "g1"); !errors.Is(err, ErrNotActive) {
				t.Fatalf("second Disable err = %v", err)
			}
			if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
				t.Fatal(err)
			}
			if f.hits.Load() != 1 {
				t.Fatalf("archive downloaded %d times, cache must be reused", f.hits.Load())
			}
		})
	}
}

func TestEnableDgVoodoo(t *testing.T) {
	ar := makeZip(t, dgvoodooEntries())
	f := newFixture(t, ar, dgvoodooRelease, machineX86, []string{"DDRAW.dll"})
	f.write(t, "dgVoodoo.conf", "USER CONF")
	f.write(t, "ddraw.dll", "OLD DDRAW")
	ctx := context.Background()
	if err := f.svc.Enable(ctx, "g1", WrapperDgVoodoo, "1080p"); err != nil {
		t.Fatal(err)
	}
	conf := f.read(t, "dgVoodoo.conf")
	if !strings.Contains(conf, "max_fhd") || !strings.Contains(conf, "dgVoodooWatermark                   = false") {
		t.Fatalf("conf = %q", conf)
	}
	if f.read(t, "ddraw.dll") != "DGV-ddraw" || f.read(t, "d3dimm.dll") != "DGV-d3dimm" {
		t.Fatal("dlls not placed")
	}
	if err := f.svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "dgVoodoo.conf") != "USER CONF" || f.read(t, "ddraw.dll") != "OLD DDRAW" || f.exists("d3dimm.dll") {
		t.Fatal("originals not restored")
	}
}

func TestDisableRefusesTamperedFile(t *testing.T) {
	ar := makeTgz(t, dxvkEntries())
	f := newFixture(t, ar, dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	ctx := context.Background()
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.write(t, "d3d9.dll", "USER REPLACED")
	err := f.svc.Disable(ctx, "g1")
	if !errors.Is(err, ErrTampered) || !strings.Contains(err.Error(), "d3d9.dll") {
		t.Fatalf("err = %v, want ErrTampered naming d3d9.dll", err)
	}
	if f.read(t, "d3d9.dll") != "USER REPLACED" {
		t.Fatal("user file was touched")
	}
	info, err := f.svc.Inspect(ctx, "g1")
	if err != nil || info.Active == nil {
		t.Fatalf("entry must stay active: %+v %v", info, err)
	}
}

func TestEnableRefusals(t *testing.T) {
	ar := makeTgz(t, dxvkEntries())
	ctx := context.Background()
	cases := []struct {
		name    string
		wrapper string
		preset  string
		game    string
		imports []string
		setup   func(f *fixture, t *testing.T)
		want    error
	}{
		{"unknown wrapper", "reshade", "default", "g1", []string{"d3d9.dll"}, nil, ErrBadWrapper},
		{"unknown preset", WrapperDXVK, "1080p", "g1", []string{"d3d9.dll"}, nil, ErrBadPreset},
		{"empty preset", WrapperDXVK, "", "g1", []string{"d3d9.dll"}, nil, ErrBadPreset},
		{"bad game id", WrapperDXVK, "default", "../x", []string{"d3d9.dll"}, nil, ErrBadGameID},
		{"nothing to replace", WrapperDXVK, "default", "g1", []string{"ddraw.dll"}, nil, ErrNoTarget},
		{"running game", WrapperDXVK, "default", "g1", []string{"d3d9.dll"}, func(f *fixture, _ *testing.T) { f.games.setRunning(true) }, ErrRunning},
		{"directory in place of dll", WrapperDXVK, "default", "g1", []string{"d3d9.dll"}, func(f *fixture, t *testing.T) {
			if err := os.Mkdir(filepath.Join(f.dir, "d3d9.dll"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, ErrSymlink},
		{"empty install dir", WrapperDXVK, "default", "g1", []string{"d3d9.dll"}, func(f *fixture, _ *testing.T) { f.games.game.InstallDir = "" }, ErrNoInstallDir},
		{"exe outside install dir", WrapperDXVK, "default", "g1", []string{"d3d9.dll"}, func(f *fixture, t *testing.T) {
			f.games.game.InstallDir = filepath.Join(f.dir, "sub")
		}, ErrOutsideDir},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, ar, dxvkRelease, machineX86, c.imports)
			if c.setup != nil {
				c.setup(f, t)
			}
			err := f.svc.Enable(ctx, c.game, c.wrapper, c.preset)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(f.svc.entries) != 0 {
				t.Fatalf("entries after refusal: %v", f.svc.entries)
			}
			if f.exists("dxgi.dll") || f.exists("d3d11.dll") {
				t.Fatal("files were written despite refusal")
			}
		})
	}
}

func TestEnableCancelledContext(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("cancelled call reached the network")
	}
}

func TestArchiveRejections(t *testing.T) {
	good := dxvkEntries()
	cases := []struct {
		name    string
		entries []archEntry
		wrongSH bool
		want    error
	}{
		{"hash mismatch", good, true, ErrArchiveHash},
		{"traversal entry", append([]archEntry{{name: "../evil.dll", data: "x"}}, good...), false, ErrBadArchive},
		{"absolute entry", append([]archEntry{{name: "/evil.dll", data: "x"}}, good...), false, ErrBadArchive},
		{"backslash entry", append([]archEntry{{name: "a\\..\\evil.dll", data: "x"}}, good...), false, ErrBadArchive},
		{"missing required entry", good[:2], false, ErrBadArchive},
		{"symlink in place of dll", append([]archEntry{{name: "dxvk-1/x32/d3d9.dll", data: "/etc/passwd", kind: tar.TypeSymlink}}, good[1:]...), false, ErrBadArchive},
		{"duplicate entry", append(append([]archEntry{}, good...), good[0]), false, ErrBadArchive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ar := makeTgz(t, c.entries)
			f := newFixture(t, ar, func(u string, a []byte) release {
				r := dxvkRelease(u, a)
				if c.wrongSH {
					r.SHA256 = strings.Repeat("0", 64)
				}
				return r
			}, machineX86, []string{"d3d9.dll"})
			err := f.svc.Enable(context.Background(), "g1", WrapperDXVK, "default")
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if f.exists("d3d9.dll") {
				t.Fatal("game dir was written")
			}
			if _, err := os.Stat(filepath.Join(f.cfg, "wrappers", "dxvk-1")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("cache published for a bad archive: %v", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(f.dir), "evil.dll")); err == nil {
				t.Fatal("entry escaped the extraction directory")
			}
		})
	}
}

func TestDownloadStatusAndRedirects(t *testing.T) {
	ar := makeTgz(t, dxvkEntries())
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("foreign host was contacted")
	}))
	t.Cleanup(other.Close)
	cases := []struct {
		name    string
		handler func(self string) http.HandlerFunc
		want    error
	}{
		{"redirect to foreign host", func(string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL+"/a", http.StatusFound) }
		}, ErrForeignHost},
		{"server error", func(string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) }
		}, ErrDownload},
		{"redirect loop", func(self string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, self+"/a", http.StatusFound) }
		}, ErrDownload},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var self string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.handler(self)(w, r) }))
			t.Cleanup(srv.Close)
			self = srv.URL
			u, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, ar, dxvkRelease, machineX86, []string{"d3d9.dll"})
			rel := dxvkRelease(srv.URL+"/a", ar)
			f.svc.releases = map[string]release{WrapperDXVK: rel}
			f.svc.dl = newDownloader(allowHost(u.Host))
			err = f.svc.Enable(context.Background(), "g1", WrapperDXVK, "default")
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestAllowedURL(t *testing.T) {
	cases := []struct {
		raw string
		ok  bool
	}{
		{"https://github.com/a/b", true},
		{"https://objects.githubusercontent.com/x", true},
		{"https://release-assets.githubusercontent.com/x", true},
		{"http://github.com/a", false},
		{"https://evil.example/a", false},
		{"https://github.com.evil.example/a", false},
		{"https://127.0.0.1/a", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := allowedURL(u) == nil; got != c.ok {
			t.Errorf("allowedURL(%s) = %v, want %v", c.raw, got, c.ok)
		}
	}
}

func TestProductionReleasesAreConsistent(t *testing.T) {
	for name, r := range productionReleases() {
		u, err := url.Parse(r.URL)
		if err != nil || allowedURL(u) != nil {
			t.Errorf("%s: url %s is not allowed: %v", name, r.URL, err)
		}
		if len(r.SHA256) != 64 || r.Version == "" || len(r.Files) == 0 || !r.hasPreset("default") {
			t.Errorf("%s: incomplete release %+v", name, r)
		}
		for _, f := range r.Files {
			if validEntryName(f.From) != nil || validEntryName(f.To) != nil {
				t.Errorf("%s: bad entry %+v", name, f)
			}
		}
	}
}

func TestNewServiceState(t *testing.T) {
	games := &fakeGames{}
	rels := map[string]release{}
	dl := newDownloader(allowedURL)
	cases := []struct {
		name  string
		setup func(t *testing.T) string
		want  error
	}{
		{"empty config dir", func(*testing.T) string { return "" }, nil},
		{"relative config dir", func(*testing.T) string { return "rel" }, nil},
		{"no state file", func(t *testing.T) string { return t.TempDir() }, nil},
		{"corrupt json", func(t *testing.T) string {
			return seedState(t, "{not json")
		}, ErrBadState},
		{"unknown version", func(t *testing.T) string {
			return seedState(t, `{"version":99,"entries":{}}`)
		}, ErrBadState},
		{"unreadable (directory)", func(t *testing.T) string {
			cfg := t.TempDir()
			if err := os.MkdirAll(filepath.Join(cfg, "wrappers", "state.json"), 0o700); err != nil {
				t.Fatal(err)
			}
			return cfg
		}, ErrBadState},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.setup(t)
			_, err := newService(cfg, games, rels, dl, true)
			wantErr := c.name != "no state file"
			if (err != nil) != wantErr {
				t.Fatalf("err = %v", err)
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	if _, err := newService(t.TempDir(), nil, rels, dl, true); err == nil {
		t.Fatal("nil games provider must be an error")
	}
}

func seedState(t *testing.T, content string) string {
	t.Helper()
	cfg := t.TempDir()
	root := filepath.Join(cfg, "wrappers")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func (f *fixture) reopen(t *testing.T) *Service {
	t.Helper()
	rels := f.svc.releases
	svc, err := newService(f.cfg, f.games, rels, f.svc.dl, true)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func (f *fixture) setPhase(t *testing.T, phase string) {
	t.Helper()
	p := filepath.Join(f.cfg, "wrappers", "state.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, old := range []string{phaseApplied, phaseRemoving, phaseApplying} {
		s = strings.ReplaceAll(s, `"phase": "`+old+`"`, `"phase": "`+phase+`"`)
	}
	if err := storage.WriteAtomic(p, []byte(s)); err != nil {
		t.Fatal(err)
	}
}

func TestStartupRecoversInterruptedOperations(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		phase  string
		damage func(f *fixture, t *testing.T)
	}{
		{"crash after replacing every file", phaseApplying, nil},
		{"crash before replacing: original still in place", phaseApplying, func(f *fixture, t *testing.T) {
			f.write(t, "d3d9.dll", "ORIGINAL")
			if err := os.Remove(filepath.Join(f.dir, "dxgi.dll")); err != nil {
				t.Fatal(err)
			}
		}},
		{"crash in the middle of removal", phaseRemoving, func(f *fixture, t *testing.T) {
			f.write(t, "d3d9.dll", "ORIGINAL")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll", "d3d11.dll"})
			f.write(t, "d3d9.dll", "ORIGINAL")
			if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
				t.Fatal(err)
			}
			f.setPhase(t, c.phase)
			if c.damage != nil {
				c.damage(f, t)
			}
			svc := f.reopen(t)
			if len(svc.entries) != 1 {
				t.Fatalf("entry must survive the restart: %v", svc.entries)
			}
			if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			if f.read(t, "d3d9.dll") != "ORIGINAL" {
				t.Fatal("original d3d9.dll not restored")
			}
			for _, n := range []string{"d3d11.dll", "d3d10core.dll", "dxgi.dll"} {
				if f.exists(n) {
					t.Fatalf("%s left behind", n)
				}
			}
			if len(svc.entries) != 0 || len(f.reopen(t).entries) != 0 {
				t.Fatal("entry not cleared in memory and on disk")
			}
			if _, err := os.Stat(filepath.Join(f.cfg, "wrappers", "backup", "g1")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("backup remains: %v", err)
			}
		})
	}
}

func TestStartupKeepsApplied(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	svc := f.reopen(t)
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil || len(svc.entries) != 1 {
		t.Fatalf("applied entry must be left alone: %v %v", err, svc.entries)
	}
}

func TestStartupRecordsFailedRecovery(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		fix   func(f *fixture, t *testing.T)
		clean bool
	}{
		{"user puts the file back", func(f *fixture, t *testing.T) { f.write(t, "d3d9.dll", "ORIGINAL") }, true},
		{"user deletes the file", func(f *fixture, t *testing.T) {
			if err := os.Remove(filepath.Join(f.dir, "d3d9.dll")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"file stays modified", func(*fixture, *testing.T) {}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
			f.write(t, "d3d9.dll", "ORIGINAL")
			if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
				t.Fatal(err)
			}
			f.setPhase(t, phaseApplying)
			f.write(t, "d3d9.dll", "USER FILE")
			svc := f.reopen(t)
			if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatalf("a failed recovery must not stop the launcher: %v", err)
			}
			if f.read(t, "d3d9.dll") != "USER FILE" {
				t.Fatal("user file was touched")
			}
			info, err := svc.Inspect(ctx, "g1")
			want := &Problem{Code: "wrappers.tampered", File: "d3d9.dll"}
			if err != nil || !reflect.DeepEqual(info.Problem, want) || info.Active != nil {
				t.Fatalf("info = %+v, err = %v", info, err)
			}
			if again := f.reopen(t); !reflect.DeepEqual(again.entries["g1"].Problem, want) {
				t.Fatalf("problem not persisted: %+v", again.entries["g1"])
			}
			if err := svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, ErrNeedsRepair) {
				t.Fatalf("Enable err = %v", err)
			}
			err = svc.Disable(ctx, "g1")
			if !c.clean {
				if !errors.Is(err, ErrTampered) || !strings.Contains(err.Error(), "d3d9.dll") || len(svc.entries) != 1 {
					t.Fatalf("Disable err = %v", err)
				}
				return
			}
			c.fix(f, t)
			if err := svc.Disable(ctx, "g1"); err != nil {
				t.Fatal(err)
			}
			if len(svc.entries) != 0 || len(f.reopen(t).entries) != 0 {
				t.Fatal("entry not removed")
			}
			info, err = svc.Inspect(ctx, "g1")
			if err != nil || info.Problem != nil || info.Active != nil {
				t.Fatalf("info = %+v, err = %v", info, err)
			}
		})
	}
}

func TestStartupReturnsJournalWriteError(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.setPhase(t, phaseApplying)
	svc := f.reopen(t)
	state := filepath.Join(f.cfg, "wrappers", "state.json")
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err == nil {
		t.Fatal("a journal that cannot be written must fail startup")
	}
}

func TestStartupDropsEntryOfDeletedGame(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.setPhase(t, phaseRemoving)
	if err := os.RemoveAll(f.dir); err != nil {
		t.Fatal(err)
	}
	f.games.game.ID = "removed-from-library"
	svc := f.reopen(t)
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(svc.entries) != 0 {
		t.Fatalf("entries = %v", svc.entries)
	}
}

func TestEnableFailedPersistTouchesNothing(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	if err := os.Mkdir(filepath.Join(f.cfg, "wrappers", "state.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Enable(context.Background(), "g1", WrapperDXVK, "default"); err == nil {
		t.Fatal("Enable must fail when the journal cannot be written")
	}
	if len(f.svc.entries) != 0 {
		t.Fatalf("memory not rolled back: %v", f.svc.entries)
	}
	if f.read(t, "d3d9.dll") != "ORIGINAL" || f.exists("dxgi.dll") {
		t.Fatal("game dir changed although the journal was not written")
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.svc.supported = false
	ctx := context.Background()
	info, err := f.svc.Inspect(ctx, "g1")
	if err != nil || info.Supported || info.Imports == nil {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Enable err = %v", err)
	}
	if err := f.svc.Disable(ctx, "g1"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Disable err = %v", err)
	}
}

func TestInspectNonPE(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := os.WriteFile(f.games.game.Executable, []byte("not an exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := f.svc.Inspect(context.Background(), "g1")
	if err != nil || info.Supported {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
}

func TestConcurrentEnableOnlyOneWins(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = f.svc.Enable(context.Background(), "g1", WrapperDXVK, "default")
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, ErrActive) && !errors.Is(err, ErrBusy):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d calls succeeded, want 1", ok)
	}
	if f.hits.Load() != 1 {
		t.Fatalf("archive downloaded %d times", f.hits.Load())
	}
}

func TestDamagedCacheIsRebuilt(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(f.cfg, "wrappers", "dxvk-1", "x86", "d3d9.dll")
	if err := storage.WriteAtomic(cached, []byte("tampered cache")); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "d3d9.dll") != "DXVK-d3d9" || f.hits.Load() != 2 {
		t.Fatalf("dll = %q, downloads = %d", f.read(t, "d3d9.dll"), f.hits.Load())
	}
	if err := f.svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(f.cfg, "wrappers", "dxvk-1", manifestName)
	if err := storage.WriteAtomic(manifest, []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil || f.hits.Load() != 3 {
		t.Fatalf("err = %v, downloads = %d", err, f.hits.Load())
	}
}
