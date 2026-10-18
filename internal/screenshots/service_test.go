package screenshots

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

type fakeGames struct {
	byPID   map[uint32]string
	running []string
	dirs    map[string]string
}

func (f fakeGames) RunningGameForPID(pid uint32) (string, bool) {
	id, ok := f.byPID[pid]
	return id, ok
}

func (f fakeGames) RunningGames() []string { return f.running }

func (f fakeGames) InstallDir(id string) (string, bool) {
	dir, ok := f.dirs[id]
	return dir, ok
}

type emitted struct {
	name    string
	payload any
}

type rig struct {
	svc     *Service
	root    string
	mu      sync.Mutex
	events  []emitted
	opened  []string
	exe     string
	exeErr  error
	tree    map[uint32]uint32
	treeErr error
}

func newRig(t *testing.T, games fakeGames) *rig {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Screenshots")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &rig{root: root}
	svc, err := NewService(func() settings.Settings {
		s := settings.Defaults()
		s.ScreenshotsPath = r.root
		return s
	}, games)
	if err != nil {
		t.Fatal(err)
	}
	svc.emit = func(name string, payload any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.events = append(r.events, emitted{name, payload})
	}
	svc.openDir = func(path string) error {
		r.opened = append(r.opened, path)
		return nil
	}
	svc.exeOf = func(context.Context, uint32) (string, error) { return r.exe, r.exeErr }
	svc.pidOf = func(hwnd uintptr) (uint32, error) {
		g, err := svc.grab(hwnd)
		return g.pid, err
	}
	svc.parents = func(context.Context) (map[uint32]uint32, error) { return r.tree, r.treeErr }
	svc.now = func() time.Time { return time.Date(2026, 10, 15, 12, 30, 45, 0, time.Local) }
	r.svc = svc
	return r
}

var (
	reds   = [4]uint8{0, 60, 120, 180}
	greens = [3]uint8{0, 80, 160}
)

func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := range 3 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: reds[x], G: greens[y], B: 200, A: 255})
		}
	}
	return img
}

func writeFile(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestNewServiceRequiresPorts(t *testing.T) {
	if _, err := NewService(nil, fakeGames{}); err == nil {
		t.Fatal("nil settings source must fail")
	}
	if _, err := NewService(func() settings.Settings { return settings.Defaults() }, nil); err == nil {
		t.Fatal("nil games port must fail")
	}
}

func TestSaveImageWritesDecodablePNG(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 10, 15, 12, 30, 45, 0, time.Local)
	path, err := saveImage(root, "game1", testImage(), at)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "game1", "2026-10-15_12-30-45.png")
	if path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatalf("saved file is not a PNG: %v", err)
	}
	if got.Bounds() != image.Rect(0, 0, 4, 3) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	r, g, b, a := got.At(2, 1).RGBA()
	if r>>8 != 120 || g>>8 != 80 || b>>8 != 200 || a>>8 != 255 {
		t.Fatalf("pixel (2,1) = %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestSaveImageSameSecondGetsNewName(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 10, 15, 12, 30, 45, 0, time.Local)
	first, err := saveImage(root, "g", testImage(), at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveImage(root, "g", testImage(), at)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("the second shot overwrote the first")
	}
	if filepath.Base(second) != "2026-10-15_12-30-45_2.png" {
		t.Fatalf("second = %s", second)
	}
}

func TestSaveImageRefusals(t *testing.T) {
	root := t.TempDir()
	at := time.Now()
	for _, id := range []string{"", "..", "a/b", `a\b`, "a..b"} {
		if _, err := saveImage(root, id, testImage(), at); !errors.Is(err, ErrBadName) {
			t.Errorf("id %q: error = %v, want ErrBadName", id, err)
		}
	}
	if _, err := saveImage(filepath.Join(root, "missing"), "g", testImage(), at); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("missing root: error = %v, want ErrNoLibrary", err)
	}
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveImage(root, "file", testImage(), at); !errors.Is(err, ErrWrite) {
		t.Errorf("game dir is a file: error = %v, want ErrWrite", err)
	}
}

func TestListMissingAndEmptyDirectory(t *testing.T) {
	r := newRig(t, fakeGames{})
	got, err := r.svc.List("nope")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("missing dir: %v, %v; want an empty list", got, err)
	}
	if err := os.Mkdir(filepath.Join(r.root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = r.svc.List("empty")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty dir: %v, %v", got, err)
	}
}

func TestListFiltersAndOrders(t *testing.T) {
	r := newRig(t, fakeGames{})
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dir := filepath.Join(r.root, "g1")
	writeFile(t, filepath.Join(dir, "a.png"), base)
	writeFile(t, filepath.Join(dir, "b.png"), base.Add(2*time.Hour))
	writeFile(t, filepath.Join(dir, "c.PNG"), base.Add(time.Hour))
	writeFile(t, filepath.Join(dir, "notes.txt"), base.Add(5*time.Hour))
	writeFile(t, filepath.Join(dir, ".tmp-123"), base.Add(6*time.Hour))
	if err := os.Mkdir(filepath.Join(dir, "sub.png"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := r.svc.List("g1")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"b.png", "c.PNG", "a.png"}) {
		t.Fatalf("names = %v, want newest first and png only", names)
	}
	if got[0].URL != "/screenshots/g1/b.png" || got[0].Path != filepath.Join(dir, "b.png") || got[0].Size != 1 {
		t.Fatalf("first = %+v", got[0])
	}
}

func TestListEscapesURL(t *testing.T) {
	r := newRig(t, fakeGames{})
	writeFile(t, filepath.Join(r.root, "g1", "shot #1 100%.png"), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))

	got, err := r.svc.List("g1")
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v, %v", got, err)
	}
	if got[0].URL != "/screenshots/g1/shot%20%231%20100%25.png" {
		t.Fatalf("url = %q", got[0].URL)
	}
	req := httptest.NewRequest(http.MethodGet, got[0].URL, nil)
	rec := httptest.NewRecorder()
	r.svc.Middleware(http.NotFoundHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("serve escaped url: status %d", rec.Code)
	}
}

func TestListRejectsBadGameID(t *testing.T) {
	r := newRig(t, fakeGames{})
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`} {
		if _, err := r.svc.List(id); !errors.Is(err, ErrBadName) {
			t.Errorf("List(%q) error = %v, want ErrBadName", id, err)
		}
	}
}

func TestListNotADirectoryIsAnError(t *testing.T) {
	r := newRig(t, fakeGames{})
	writeFile(t, filepath.Join(r.root, "file"), time.Now())
	if _, err := r.svc.List("file"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want a real read failure", err)
	}
}

func TestNoLibraryIsReported(t *testing.T) {
	svc, err := NewService(func() settings.Settings { return settings.Defaults() }, fakeGames{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List("g"); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("List: %v", err)
	}
	if err := svc.Delete("g", "a.png"); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.OpenFolder("g"); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("OpenFolder: %v", err)
	}
	if _, err := svc.capture(context.Background(), 1); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("capture: %v", err)
	}
	if uierr.Code(ErrNoLibrary) != "screenshots.no_library" {
		t.Fatalf("code = %q", uierr.Code(ErrNoLibrary))
	}
}

func TestDelete(t *testing.T) {
	r := newRig(t, fakeGames{})
	keep := filepath.Join(r.root, "g1", "keep.png")
	gone := filepath.Join(r.root, "g1", "gone.png")
	outside := filepath.Join(filepath.Dir(r.root), "outside.png")
	writeFile(t, keep, time.Now())
	writeFile(t, gone, time.Now())
	writeFile(t, outside, time.Now())

	if err := r.svc.Delete("g1", "gone.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file still there: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("neighbour removed: %v", err)
	}
	if err := r.svc.Delete("g1", "gone.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleting twice: %v, want ErrNotExist", err)
	}

	cases := [][2]string{
		{"g1", "../../outside.png"},
		{"g1", "..\\..\\outside.png"},
		{"..", "outside.png"},
		{"g1/..", "outside.png"},
		{"g1", "sub/a.png"},
		{"g1", "keep.txt"},
		{"g1", ""},
		{"", "keep.png"},
	}
	for _, c := range cases {
		if err := r.svc.Delete(c[0], c[1]); !errors.Is(err, ErrBadName) {
			t.Errorf("Delete(%q, %q) = %v, want ErrBadName", c[0], c[1], err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside the screenshots folder was touched: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("keep.png was touched: %v", err)
	}
}

func TestOpenFolder(t *testing.T) {
	r := newRig(t, fakeGames{})
	if err := r.svc.OpenFolder("g1"); !errors.Is(err, ErrNoShots) {
		t.Fatalf("no folder: %v, want ErrNoShots", err)
	}
	if len(r.opened) != 0 {
		t.Fatal("a missing folder must not be created or opened")
	}
	if _, err := os.Stat(filepath.Join(r.root, "g1")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("OpenFolder created the folder: %v", err)
	}
	writeFile(t, filepath.Join(r.root, "g1", "a.png"), time.Now())
	if err := r.svc.OpenFolder("g1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.opened, []string{filepath.Join(r.root, "g1")}) {
		t.Fatalf("opened %v", r.opened)
	}
	if err := r.svc.OpenFolder("../g1"); !errors.Is(err, ErrBadName) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestStatus(t *testing.T) {
	r := newRig(t, fakeGames{})
	st := r.svc.Status()
	if !st.Enabled || st.Hotkey != settings.ScreenshotHotkeyCtrlF12 || st.Dir != r.root {
		t.Fatalf("status = %+v", st)
	}
}

func TestMiddleware(t *testing.T) {
	r := newRig(t, fakeGames{})
	writeFile(t, filepath.Join(r.root, "g1", "a.png"), time.Now())
	writeFile(t, filepath.Join(r.root, "g1", "a.txt"), time.Now())
	writeFile(t, filepath.Join(filepath.Dir(r.root), "secret.png"), time.Now())

	var passed bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		passed = true
		w.WriteHeader(http.StatusTeapot)
	})
	h := r.svc.Middleware(next)

	cases := []struct {
		name   string
		method string
		path   string
		status int
		next   bool
	}{
		{"existing shot", http.MethodGet, "/screenshots/g1/a.png", http.StatusOK, false},
		{"head", http.MethodHead, "/screenshots/g1/a.png", http.StatusOK, false},
		{"missing file", http.MethodGet, "/screenshots/g1/none.png", http.StatusNotFound, false},
		{"not a png", http.MethodGet, "/screenshots/g1/a.txt", http.StatusNotFound, false},
		{"parent in game id", http.MethodGet, "/screenshots/../secret.png", http.StatusNotFound, false},
		{"parent in name", http.MethodGet, "/screenshots/g1/..%2Fsecret.png", http.StatusNotFound, false},
		{"parent as game id", http.MethodGet, "/screenshots/%2e%2e/secret.png", http.StatusNotFound, false},
		{"extra segment", http.MethodGet, "/screenshots/g1/sub/a.png", http.StatusNotFound, false},
		{"backslash", http.MethodGet, "/screenshots/g1/..%5Csecret.png", http.StatusNotFound, false},
		{"no name", http.MethodGet, "/screenshots/g1", http.StatusNotFound, false},
		{"post", http.MethodPost, "/screenshots/g1/a.png", http.StatusNotFound, false},
		{"delete", http.MethodDelete, "/screenshots/g1/a.png", http.StatusNotFound, false},
		{"other prefix", http.MethodGet, "/media/x.jpg", http.StatusTeapot, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			passed = false
			req := httptest.NewRequest(c.method, c.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.status || passed != c.next {
				t.Fatalf("status = %d (want %d), passed to next = %v (want %v)", rec.Code, c.status, passed, c.next)
			}
		})
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/screenshots/g1/a.png", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type = %q", ct)
	}
}

func TestMiddlewareWithoutLibrary(t *testing.T) {
	svc, err := NewService(func() settings.Settings { return settings.Defaults() }, fakeGames{})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	svc.Middleware(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/screenshots/g1/a.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPickGame(t *testing.T) {
	install := filepath.Join(t.TempDir(), "games", "b")
	inside := filepath.Join(install, "bin", "game.exe")
	sibling := install + "-other"
	cases := []struct {
		name    string
		games   fakeGames
		pid     uint32
		exe     string
		exeErr  error
		want    string
		wantErr error
	}{
		{"by pid", fakeGames{byPID: map[uint32]string{10: "a"}, running: []string{"a", "b"}}, 10, "", nil, "a", nil},
		{"pid wins over single running", fakeGames{byPID: map[uint32]string{10: "a"}, running: []string{"b"}}, 10, "", nil, "a", nil},
		{"single running, exe inside install dir", fakeGames{running: []string{"b"}, dirs: map[string]string{"b": install}}, 99, inside, nil, "b", nil},
		{"single running, exe outside install dir", fakeGames{running: []string{"b"}, dirs: map[string]string{"b": install}}, 99, filepath.Join(filepath.Dir(install), "browser.exe"), nil, "", ErrNoGame},
		{"single running, exe in a sibling folder", fakeGames{running: []string{"b"}, dirs: map[string]string{"b": install}}, 99, filepath.Join(sibling, "game.exe"), nil, "", ErrNoGame},
		{"single running, exe unreadable", fakeGames{running: []string{"b"}, dirs: map[string]string{"b": install}}, 99, "", errors.New("access denied"), "", ErrNoGame},
		{"single running, no install dir", fakeGames{running: []string{"b"}}, 99, inside, nil, "", ErrNoGame},
		{"two running, pid unknown", fakeGames{running: []string{"a", "b"}, dirs: map[string]string{"b": install}}, 99, inside, nil, "", ErrNoGame},
		{"nothing running", fakeGames{}, 99, inside, nil, "", ErrNoGame},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.games)
			r.exe, r.exeErr = c.exe, c.exeErr
			got, err := r.svc.pickGame(context.Background(), c.pid)
			if got != c.want || !errors.Is(err, c.wantErr) {
				t.Fatalf("pickGame = %q, %v; want %q, %v", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestPickGameClimbsParents(t *testing.T) {
	install := filepath.Join(t.TempDir(), "games", "cg")
	temp := filepath.Join(t.TempDir(), "Temp", "codenamegordon.exe")
	games := fakeGames{byPID: map[uint32]string{100: "cg"}, running: []string{"cg"}, dirs: map[string]string{"cg": install}}
	cases := []struct {
		name    string
		tree    map[uint32]uint32
		treeErr error
		pid     uint32
		want    string
		wantErr error
	}{
		{"child of the session pid", map[uint32]uint32{200: 100, 100: 4}, nil, 200, "cg", nil},
		{"grandchild of the session pid", map[uint32]uint32{300: 200, 200: 100}, nil, 300, "cg", nil},
		{"unrelated process tree", map[uint32]uint32{200: 150, 150: 4}, nil, 200, "", ErrNoGame},
		{"parent loop does not hang", map[uint32]uint32{200: 201, 201: 200}, nil, 200, "", ErrNoGame},
		{"snapshot failure refuses and keeps the cause", nil, errors.New("snapshot denied"), 200, "", ErrNoGame},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, games)
			r.exe = temp
			r.tree, r.treeErr = c.tree, c.treeErr
			got, err := r.svc.pickGame(context.Background(), c.pid)
			if got != c.want || !errors.Is(err, c.wantErr) {
				t.Fatalf("pickGame = %q, %v; want %q, %v", got, err, c.want, c.wantErr)
			}
			if c.wantErr == nil {
				return
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", temp)) {
				t.Fatalf("refusal %q does not name the window executable %q", err, temp)
			}
			if c.treeErr != nil && !errors.Is(err, c.treeErr) {
				t.Fatalf("refusal %v lost the snapshot error", err)
			}
		})
	}
}

func grabber(pid uint32, err error) func(uintptr) (grabbed, error) {
	return func(uintptr) (grabbed, error) {
		if err != nil {
			return grabbed{}, err
		}
		return grabbed{img: testImage(), pid: pid}, nil
	}
}

func TestCaptureStoresUnderTheGame(t *testing.T) {
	r := newRig(t, fakeGames{byPID: map[uint32]string{10: "g1"}})
	r.svc.grab = grabber(10, nil)
	r.svc.SetTitleResolver(func(id string) (string, bool) { return "Title " + id, true })

	saved, err := r.svc.capture(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if saved.GameID != "g1" || saved.Title != "Title g1" || saved.Name != "2026-10-15_12-30-45.png" {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := os.Stat(saved.Path); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureOfAForeignWindowStoresNothing(t *testing.T) {
	install := filepath.Join(t.TempDir(), "game")
	r := newRig(t, fakeGames{running: []string{"a"}, dirs: map[string]string{"a": install}})
	r.svc.pidOf = func(uintptr) (uint32, error) { return 99, nil }
	grabs := 0
	r.svc.grab = func(uintptr) (grabbed, error) {
		grabs++
		return grabbed{img: testImage(), pid: 99}, nil
	}
	r.exe = filepath.Join(t.TempDir(), "browser.exe")
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	r.svc.Capture(1)
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}

	if grabs != 0 {
		t.Fatalf("a foreign window was grabbed %d times before attribution refused it", grabs)
	}
	entries, err := os.ReadDir(r.root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("files left in the screenshots folder: %v %v", entries, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) != 0 {
		t.Fatalf("events = %+v, want none", r.events)
	}
}

func TestParallelCapturesGetDistinctFiles(t *testing.T) {
	r := newRig(t, fakeGames{running: []string{"g1"}, byPID: map[uint32]string{1: "g1"}})
	r.svc.grab = grabber(1, nil)
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	const n = 8
	for range n {
		r.svc.Capture(1)
	}
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	paths := map[string]bool{}
	for _, e := range r.events {
		if saved, ok := e.payload.(Saved); ok {
			paths[saved.Path] = true
		}
	}
	if len(r.events) != n || len(paths) != n {
		t.Fatalf("%d events, %d distinct paths, want %d of each", len(r.events), len(paths), n)
	}
	entries, err := os.ReadDir(filepath.Join(r.root, "g1"))
	if err != nil || len(entries) != n {
		t.Fatalf("%d files on disk (%v), want %d", len(entries), err, n)
	}
}

func TestSessionsArmTheHotkey(t *testing.T) {
	r := newRig(t, fakeGames{})
	var calls []bool
	r.svc.SetArmer(func(armed bool) { calls = append(calls, armed) })

	r.svc.SessionStopped("never-started")
	if len(calls) != 0 {
		t.Fatalf("a stop without a start armed or disarmed: %v", calls)
	}
	r.svc.SessionStarted(library.Game{ID: "a"})
	r.svc.SessionStarted(library.Game{ID: "b"})
	r.svc.SessionStarted(library.Game{ID: "b"})
	if !slices.Equal(calls, []bool{true}) {
		t.Fatalf("calls after two sessions = %v, want one arm", calls)
	}
	r.svc.SessionStopped("a")
	if !slices.Equal(calls, []bool{true}) {
		t.Fatalf("calls with one session left = %v", calls)
	}
	r.svc.SessionStopped("b")
	if !slices.Equal(calls, []bool{true, false}) {
		t.Fatalf("calls after both ended = %v", calls)
	}
	r.svc.SessionStopped("b")
	if len(calls) != 2 {
		t.Fatalf("a repeated stop disarmed again: %v", calls)
	}
}

func TestInsideDir(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		dir, path string
		want      bool
	}{
		{base, filepath.Join(base, "a.exe"), true},
		{base, filepath.Join(base, "sub", "a.exe"), true},
		{base, base, true},
		{base, filepath.Join(filepath.Dir(base), "a.exe"), false},
		{base, base + "x", false},
		{base, filepath.Join(base, "..", "a.exe"), false},
		{"", filepath.Join(base, "a.exe"), false},
		{base, "", false},
		{"games", "games/a.exe", false},
	}
	for _, c := range cases {
		if got := insideDir(c.dir, c.path); got != c.want {
			t.Errorf("insideDir(%q, %q) = %v, want %v", c.dir, c.path, got, c.want)
		}
	}
}

func TestCaptureReportsDistinctFailures(t *testing.T) {
	cases := []struct {
		name string
		grab func(uintptr) (grabbed, error)
		want error
	}{
		{"no window", grabber(0, ErrNoWindow), ErrNoWindow},
		{"gdi", grabber(0, ErrGDI), ErrGDI},
		{"unsupported", grabber(0, ErrUnsupported), ErrUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, fakeGames{})
			r.svc.grab = c.grab
			_, err := r.svc.capture(context.Background(), 1)
			if !errors.Is(err, c.want) || errors.Is(err, ErrNoGame) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			entries, readErr := os.ReadDir(r.root)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("a failed capture left files behind: %v %v", entries, readErr)
			}
		})
	}
}

func TestCaptureCancelledContextStoresNothing(t *testing.T) {
	r := newRig(t, fakeGames{running: []string{"a"}})
	r.svc.grab = grabber(1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.svc.capture(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	entries, err := os.ReadDir(r.root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("files left: %v %v", entries, err)
	}
}

func TestCaptureEmitsThroughTheServiceGoroutine(t *testing.T) {
	r := newRig(t, fakeGames{byPID: map[uint32]string{1: "g1"}})
	r.svc.grab = grabber(1, nil)
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	r.svc.Capture(1)
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) != 1 || r.events[0].name != EventSaved {
		t.Fatalf("events = %+v", r.events)
	}
	if got, ok := r.events[0].payload.(Saved); !ok || got.GameID != "g1" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestCaptureEmitsFailure(t *testing.T) {
	r := newRig(t, fakeGames{})
	r.svc.grab = grabber(0, ErrNoWindow)
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	r.svc.Capture(1)
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) != 1 || r.events[0].name != EventFailed {
		t.Fatalf("events = %+v", r.events)
	}
}

func TestCaptureAfterShutdownIsIgnored(t *testing.T) {
	r := newRig(t, fakeGames{byPID: map[uint32]string{1: "g1"}})
	r.svc.grab = grabber(1, nil)
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	r.svc.Capture(1)
	r.svc.wg.Wait()
	if len(r.events) != 0 {
		t.Fatalf("events after shutdown: %+v", r.events)
	}
}

func TestCaptureBeforeStartupIsIgnored(t *testing.T) {
	r := newRig(t, fakeGames{byPID: map[uint32]string{1: "g1"}})
	r.svc.grab = grabber(1, nil)
	r.svc.Capture(1)
	r.svc.wg.Wait()
	if len(r.events) != 0 {
		t.Fatalf("events before startup: %+v", r.events)
	}
}
