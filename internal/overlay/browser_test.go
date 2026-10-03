package overlay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const browserHwnd = 950

type fakeBrowser struct {
	mu       sync.Mutex
	log      *eventLog
	placeErr error
	visible  bool
	placed   []rect
	urls     []string
}

func (b *fakeBrowser) handle() uintptr { return browserHwnd }

func (b *fakeBrowser) place(r rect) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.placed = append(b.placed, r)
	if b.placeErr != nil {
		return b.placeErr
	}
	b.visible = true
	b.log.add("browser:place")
	return nil
}

func (b *fakeBrowser) hide() {
	b.mu.Lock()
	b.visible = false
	b.mu.Unlock()
	b.log.add("browser:hide")
}

func (b *fakeBrowser) navigate(target string) {
	b.mu.Lock()
	b.urls = append(b.urls, target)
	b.mu.Unlock()
}

func (b *fakeBrowser) back()    { b.log.add("browser:back") }
func (b *fakeBrowser) forward() { b.log.add("browser:forward") }
func (b *fakeBrowser) reload()  { b.log.add("browser:reload") }

func (b *fakeBrowser) isVisible() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.visible
}

func newBrowserRig(t *testing.T) (*rig, *fakeBrowser, *int) {
	t.Helper()
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	b := &fakeBrowser{log: r.log}
	created := 0
	r.svc.makeBrowser = func() (browserWindow, error) {
		created++
		return b, nil
	}
	return r, b, &created
}

func TestBrowserURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"example.com", "https://example.com"},
		{"  youtube.com/watch?v=1  ", "https://youtube.com/watch?v=1"},
		{"https://twitch.tv/x", "https://twitch.tv/x"},
		{"http://example.com", "http://example.com"},
		{"localhost:8080", "https://localhost:8080"},
		{"гайд по боссу", searchURL + "%D0%B3%D0%B0%D0%B9%D0%B4+%D0%BF%D0%BE+%D0%B1%D0%BE%D1%81%D1%81%D1%83"},
		{"silksong", searchURL + "silksong"},
		{"javascript:alert(1)", searchURL + "javascript%3Aalert%281%29"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := browserURL(c.in)
			if err != nil {
				t.Fatalf("browserURL(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("browserURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBrowserURLRefusals(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"empty", "   "},
		{"file scheme", "file:///C:/Windows/win.ini"},
		{"custom scheme", "steam://run/730"},
		{"no host", "https://"},
		{"control character", "example.com/\x00"},
		{"too long", "https://example.com/" + strings.Repeat("a", browserMaxURL)},
		{"launcher assets", "http://wails.localhost/"},
		{"launcher assets without scheme", "wails.localhost/index.html"},
		{"launcher assets subdomain", "https://x.wails.localhost/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := browserURL(c.in)
			if err == nil {
				t.Fatalf("browserURL(%q) = %q, want an error", c.in, got)
			}
			if code := uierr.Code(err); code != ErrCodeBrowserURL {
				t.Fatalf("code = %q, want %q", code, ErrCodeBrowserURL)
			}
		})
	}
}

func TestGuardAssetsRefusesTheBrowserWindow(t *testing.T) {
	cases := []struct {
		window   string
		wantCode int
		wantNext bool
	}{
		{BrowserWindowName, http.StatusForbidden, false},
		{WindowName, http.StatusOK, true},
		{"", http.StatusOK, true},
	}
	for _, c := range cases {
		t.Run(c.window, func(t *testing.T) {
			reached := false
			h := GuardAssets(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			req := httptest.NewRequest(http.MethodPost, "http://wails.localhost/wails/runtime", nil)
			if c.window != "" {
				req.Header.Set(windowNameHeader, c.window)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.wantCode || reached != c.wantNext {
				t.Fatalf("code = %d, reached = %v; want %d, %v", rec.Code, reached, c.wantCode, c.wantNext)
			}
		})
	}
}

func TestOpenBrowserPlacesItInsideTheOverlay(t *testing.T) {
	r, b, created := newBrowserRig(t)
	r.svc.toggleOnUI()

	got, err := r.svc.OpenBrowser("example.com", Bounds{X: 10, Y: 60, Width: 1600, Height: 1300})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com" {
		t.Fatalf("opened %q", got)
	}
	want := rect{x: 1920 + 10, y: 60, w: 1600, h: 1300}
	if !slices.Equal(b.placed, []rect{want}) {
		t.Fatalf("placed at %+v, want %+v", b.placed, want)
	}
	if !slices.Equal(b.urls, []string{"https://example.com"}) {
		t.Fatalf("navigated to %v", b.urls)
	}

	if _, err := r.svc.OpenBrowser("twitch.tv", Bounds{X: 0, Y: 0, Width: 100, Height: 100}); err != nil {
		t.Fatal(err)
	}
	if *created != 1 {
		t.Fatalf("browser window created %d times, want once", *created)
	}
}

func TestOpenBrowserRefusals(t *testing.T) {
	good := Bounds{X: 0, Y: 0, Width: 800, Height: 600}
	cases := []struct {
		name    string
		show    bool
		address string
		area    Bounds
		want    error
	}{
		{"overlay closed", false, "example.com", good, errNotShown},
		{"empty area", true, "example.com", Bounds{Width: 0, Height: 600}, errBadBounds},
		{"negative origin", true, "example.com", Bounds{X: -1, Width: 10, Height: 10}, errBadBounds},
		{"wider than the overlay", true, "example.com", Bounds{X: 2000, Width: 600, Height: 10}, errBadBounds},
		{"taller than the overlay", true, "example.com", Bounds{Y: 1000, Width: 10, Height: 500}, errBadBounds},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, b, created := newBrowserRig(t)
			if c.show {
				r.svc.toggleOnUI()
			}
			_, err := r.svc.OpenBrowser(c.address, c.area)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if *created != 0 || len(b.placed) != 0 {
				t.Fatalf("browser touched: created %d, placed %v", *created, b.placed)
			}
		})
	}
}

func TestOpenBrowserWithoutAWindowFactory(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); !errors.Is(err, errBrowserUnsupported) {
		t.Fatalf("err = %v, want %v", err, errBrowserUnsupported)
	}
}

func TestFailedPlacementLeavesTheBrowserClosed(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	b.placeErr = errors.New("SetWindowPos failed")

	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); !errors.Is(err, b.placeErr) {
		t.Fatalf("err = %v, want %v", err, b.placeErr)
	}
	if b.isVisible() {
		t.Fatal("browser is visible after a failed placement")
	}
	if err := r.svc.BrowserBack(); !errors.Is(err, errNotShown) {
		t.Fatalf("back after failed open: %v, want %v", err, errNotShown)
	}
}

func TestHidingTheOverlayHidesTheBrowserUntilThePanelPlacesItAgain(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	area := Bounds{Width: 800, Height: 600}
	if _, err := r.svc.OpenBrowser("example.com", area); err != nil {
		t.Fatal(err)
	}

	r.svc.toggleOnUI()
	if b.isVisible() {
		t.Fatal("browser stayed on screen after the overlay closed")
	}
	if err := r.svc.PlaceBrowser(area); !errors.Is(err, errNotShown) {
		t.Fatalf("place while the overlay is closed: %v, want %v", err, errNotShown)
	}

	r.plat.setFg(gameHwnd)
	r.svc.toggleOnUI()
	if err := r.svc.PlaceBrowser(area); err != nil {
		t.Fatal(err)
	}
	if !b.isVisible() {
		t.Fatal("browser did not come back with the overlay")
	}
	if len(b.urls) != 1 {
		t.Fatalf("page reloaded on reopen: %v", b.urls)
	}
}

func TestClosedBrowserIsNotPlacedAgain(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	area := Bounds{Width: 800, Height: 600}
	if _, err := r.svc.OpenBrowser("example.com", area); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.CloseBrowser(); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.PlaceBrowser(area); err != nil {
		t.Fatal(err)
	}
	if b.isVisible() || len(b.placed) != 1 {
		t.Fatalf("closed browser placed again: visible %v, placed %v", b.isVisible(), b.placed)
	}
	if err := r.svc.BrowserReload(); !errors.Is(err, errNotShown) {
		t.Fatalf("reload of a closed browser: %v, want %v", err, errNotShown)
	}
}

func TestBrowserNavigation(t *testing.T) {
	r, _, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []func() error{r.svc.BrowserBack, r.svc.BrowserForward, r.svc.BrowserReload} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []string{"browser:back", "browser:forward", "browser:reload"} {
		if r.log.count(e) != 1 {
			t.Fatalf("%s ran %d times: %v", e, r.log.count(e), r.log.all())
		}
	}
}

func TestFocusMovingToTheBrowserKeepsTheOverlayOpen(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); err != nil {
		t.Fatal(err)
	}
	r.plat.setFg(browserHwnd)
	r.svc.lostFocus()
	if visible, _ := r.state(); !visible || !b.isVisible() {
		t.Fatalf("overlay visible %v, browser visible %v; both must stay", visible, b.isVisible())
	}

	r.plat.setFg(otherHwnd)
	r.svc.lostFocus()
	if visible, _ := r.state(); visible || b.isVisible() {
		t.Fatalf("overlay visible %v, browser visible %v after the user left", visible, b.isVisible())
	}
}
