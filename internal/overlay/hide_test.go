package overlay

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"typhon/internal/settings"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	out := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return out
}

const never = 1 << 20

func TestHideKeepsTheStateWhileTheOverlayStaysOnScreen(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	r.win.ignored = never
	before := r.log.count("emit:" + EventHidden)

	r.svc.Hide()

	if visible, _ := r.state(); !visible {
		t.Fatal("state says closed while the overlay window is still on screen")
	}
	if !r.svc.Visible() {
		t.Fatal("Visible reports closed while the overlay window is still on screen")
	}
	if got := r.log.count("emit:"+EventHidden) - before; got != 0 {
		t.Fatalf("hidden emitted %d times for a window that did not hide", got)
	}
	if got := r.plat.foregroundCalls(); len(got) != 0 {
		t.Fatalf("focus handed back to %v while the overlay is still up", got)
	}
	if got := r.win.hideCalls(); got != 2 {
		t.Fatalf("overlay hide attempted %d times, want 2 (one retry)", got)
	}

	r.win.ignored = 0
	r.svc.toggleOnUI()

	if r.win.isVisible() {
		t.Fatal("the next toggle did not take the overlay down")
	}
	if n := r.log.count("emit:" + EventShown); n != 1 {
		t.Fatalf("shown emitted %d times, the next toggle must hide, not show", n)
	}
	if visible, _ := r.state(); visible {
		t.Fatal("state still open after the overlay hid")
	}
}

func TestHideRetriesOnceAndLogsWhatItSaw(t *testing.T) {
	out := captureLog(t)
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	r.win.ignored = 1

	r.svc.Hide()

	if r.win.isVisible() {
		t.Fatal("overlay still on screen after the retry")
	}
	if got := r.win.hideCalls(); got != 2 {
		t.Fatalf("overlay hide attempted %d times, want 2", got)
	}
	if visible, _ := r.state(); visible {
		t.Fatal("state still open although the retry hid the window")
	}
	if n := r.log.count("emit:" + EventHidden); n != 1 {
		t.Fatalf("hidden emitted %d times, want 1", n)
	}
	text := out.String()
	for _, want := range []string{"level=WARN", "hwnd=900", "still on screen"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log %q lacks %q", text, want)
		}
	}
}

func TestHideKeepsTheStateWhileTheBrowserStaysOnScreen(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); err != nil {
		t.Fatal(err)
	}
	b.ignored = never
	before := r.log.count("emit:" + EventHidden)

	r.svc.Hide()

	if visible, _ := r.state(); !visible {
		t.Fatal("state says closed while the browser window is still on screen")
	}
	if r.win.isVisible() {
		t.Fatal("the overlay window itself was left up because of the browser")
	}
	if got := r.log.count("emit:"+EventHidden) - before; got != 0 {
		t.Fatalf("hidden emitted %d times for a browser that did not hide", got)
	}
	if got := b.hideCalls(); got != 2 {
		t.Fatalf("browser hide attempted %d times, want 2 (one retry)", got)
	}

	b.ignored = 0
	r.svc.toggleOnUI()

	if b.isVisible() || r.win.isVisible() {
		t.Fatalf("browser visible %v, overlay visible %v after the next toggle", b.isVisible(), r.win.isVisible())
	}
	if visible, _ := r.state(); visible {
		t.Fatal("state still open after both windows hid")
	}
	if n := r.log.count("emit:" + EventShown); n != 1 {
		t.Fatalf("shown emitted %d times, the next toggle must hide, not show", n)
	}
}

func TestToggleHidesAnOverlayThatIsOnScreenWhileTheStateSaysClosed(t *testing.T) {
	cases := []struct {
		name   string
		revive func(*rig, *fakeBrowser)
	}{
		{"overlay window came back", func(r *rig, _ *fakeBrowser) { r.win.reappear() }},
		{"browser window came back", func(_ *rig, b *fakeBrowser) { b.mu.Lock(); b.visible = true; b.mu.Unlock() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, b, _ := newBrowserRig(t)
			r.svc.toggleOnUI()
			if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); err != nil {
				t.Fatal(err)
			}
			r.svc.toggleOnUI()
			if r.win.isVisible() || b.isVisible() {
				t.Fatal("setup: windows should be hidden")
			}
			c.revive(r, b)
			shown := len(r.win.shown)

			r.svc.toggleOnUI()

			if len(r.win.shown) != shown {
				t.Fatal("toggle showed the overlay again over a window that never left the screen")
			}
			if r.win.isVisible() || b.isVisible() {
				t.Fatalf("overlay visible %v, browser visible %v after the toggle", r.win.isVisible(), b.isVisible())
			}
			if visible, _ := r.state(); visible {
				t.Fatal("state open after the windows were taken down")
			}
		})
	}
}

func TestFailedShowDoesNotClaimClosedWhileTheWindowIsUp(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.win.noFocus = true
	r.win.ignored = never

	r.svc.toggleOnUI()

	if !r.win.isVisible() {
		t.Fatal("setup: the window should be stuck on screen")
	}
	if visible, _ := r.state(); !visible {
		t.Fatal("state says closed while the window is on screen")
	}
	if n := r.log.count("emit:" + EventShown); n != 0 {
		t.Fatalf("shown emitted %d times for a show that got no focus", n)
	}

	r.win.ignored = 0
	r.svc.toggleOnUI()
	if r.win.isVisible() {
		t.Fatal("the next toggle did not take the stuck window down")
	}
}

func TestBrowserThatCannotBeUnloadedStaysOpen(t *testing.T) {
	r, b, _ := newBrowserRig(t)
	r.svc.toggleOnUI()
	if _, err := r.svc.OpenBrowser("example.com", Bounds{Width: 10, Height: 10}); err != nil {
		t.Fatal(err)
	}
	b.ignored = never

	if err := r.svc.CloseBrowser(); err != nil {
		t.Fatal(err)
	}

	if err := r.svc.BrowserReload(); errors.Is(err, errNotShown) {
		t.Fatal("browser is reported closed while its window is on screen")
	}
	if got := b.hideCalls(); got != 2 {
		t.Fatalf("browser hide attempted %d times, want 2 (one retry)", got)
	}
	if n := r.log.count("emit:" + EventBrowserClosed); n != 0 {
		t.Fatalf("browser-closed emitted %d times for a window that is still on screen", n)
	}
}
