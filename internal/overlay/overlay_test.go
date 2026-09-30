package overlay

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const (
	gameHwnd    = 100
	otherHwnd   = 300
	overlayHwnd = 900
)

type fakeRegistration struct {
	name    string
	stopped bool
}

type fakePlatform struct {
	mu            sync.Mutex
	log           *eventLog
	unsupported   bool
	fg            uintptr
	area          rect
	areaErr       error
	foregroundErr error
	live          map[uintptr]bool
	iconic        map[uintptr]bool
	state         int
	stateErr      error
	registerErr   map[string]error
	regs          []*fakeRegistration
	monitorFor    []uintptr
	foregroundSet []uintptr
	press         func()
}

func (p *fakePlatform) supported() bool { return !p.unsupported }

func (p *fakePlatform) register(_ context.Context, _ *sync.WaitGroup, key hotkey, onPress func()) (func(), error) {
	name := ""
	for n, k := range hotkeys {
		if k == key {
			name = n
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.registerErr[name]; err != nil {
		return nil, err
	}
	r := &fakeRegistration{name: name}
	p.regs = append(p.regs, r)
	p.press = onPress
	return func() {
		p.mu.Lock()
		r.stopped = true
		p.mu.Unlock()
	}, nil
}

func (p *fakePlatform) active() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, r := range p.regs {
		if !r.stopped {
			out = append(out, r.name)
		}
	}
	return out
}

func (p *fakePlatform) registrations() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.regs)
}

func (p *fakePlatform) foreground() uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fg
}

func (p *fakePlatform) setFg(hwnd uintptr) {
	p.mu.Lock()
	p.fg = hwnd
	p.mu.Unlock()
}

func (p *fakePlatform) monitorRect(hwnd uintptr) (rect, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.monitorFor = append(p.monitorFor, hwnd)
	return p.area, p.areaErr
}

func (p *fakePlatform) setForeground(hwnd uintptr) error {
	p.mu.Lock()
	p.foregroundSet = append(p.foregroundSet, hwnd)
	err := p.foregroundErr
	if err == nil {
		p.fg = hwnd
	}
	p.mu.Unlock()
	p.log.add(fmt.Sprintf("foreground:%d", hwnd))
	return err
}

func (p *fakePlatform) isWindow(hwnd uintptr) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.live[hwnd]
}

func (p *fakePlatform) notificationState() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, p.stateErr
}

func (p *fakePlatform) isIconic(hwnd uintptr) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.iconic[hwnd]
}

func (p *fakePlatform) restore(hwnd uintptr) {
	p.mu.Lock()
	delete(p.iconic, hwnd)
	p.mu.Unlock()
	p.log.add(fmt.Sprintf("restore:%d", hwnd))
}

func (p *fakePlatform) foregroundCalls() []uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.foregroundSet)
}

type eventLog struct {
	mu      sync.Mutex
	entries []string
	shown   []Shown
}

func (l *eventLog) addShown(s Shown) {
	l.mu.Lock()
	l.shown = append(l.shown, s)
	l.mu.Unlock()
}

func (l *eventLog) shownPayloads() []Shown {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.shown)
}

func (l *eventLog) add(s string) {
	l.mu.Lock()
	l.entries = append(l.entries, s)
	l.mu.Unlock()
}

func (l *eventLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

func (l *eventLog) count(s string) int {
	n := 0
	for _, e := range l.all() {
		if e == s {
			n++
		}
	}
	return n
}

type fakeWindow struct {
	mu      sync.Mutex
	log     *eventLog
	plat    *fakePlatform
	showErr error
	noFocus bool
	visible bool
	shown   []rect
}

func (w *fakeWindow) handle() uintptr { return overlayHwnd }

func (w *fakeWindow) show(r rect) error {
	w.mu.Lock()
	w.shown = append(w.shown, r)
	err := w.showErr
	if err == nil {
		w.visible = true
	}
	w.mu.Unlock()
	if err == nil && !w.noFocus {
		w.plat.setFg(overlayHwnd)
	}
	w.log.add("window:show")
	return err
}

func (w *fakeWindow) hide() {
	w.mu.Lock()
	w.visible = false
	w.mu.Unlock()
	w.log.add("window:hide")
}

func (w *fakeWindow) isVisible() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.visible
}

type rig struct {
	svc  *Service
	plat *fakePlatform
	win  *fakeWindow
	log  *eventLog
	ui   sync.Mutex
}

func newRig(t *testing.T, enabled bool, hotkeyName string) *rig {
	t.Helper()
	r := &rig{log: &eventLog{}}
	r.plat = &fakePlatform{
		log: r.log, fg: gameHwnd, area: rect{x: 1920, y: 0, w: 2560, h: 1440},
		live: map[uintptr]bool{gameHwnd: true, otherHwnd: true, overlayHwnd: true}, registerErr: map[string]error{},
		iconic: map[uintptr]bool{},
	}
	r.win = &fakeWindow{log: r.log, plat: r.plat}
	dispatch := func(f func()) {
		r.ui.Lock()
		defer r.ui.Unlock()
		f()
	}
	emit := func(name string, payload any) {
		if shown, ok := payload.(Shown); ok {
			r.log.addShown(shown)
		}
		r.log.add("emit:" + name)
	}
	svc, err := newService(r.plat, dispatch, emit, enabled, hotkeyName)
	if err != nil {
		t.Fatal(err)
	}
	svc.win = r.win
	r.svc = svc
	return r
}

func (r *rig) start(t *testing.T) {
	t.Helper()
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.svc.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
}

func (r *rig) apply(prevEnabled bool, prevKey string, enabled bool, key string) error {
	prev, next := settings.Defaults(), settings.Defaults()
	prev.OverlayEnabled, prev.OverlayHotkey = prevEnabled, prevKey
	next.OverlayEnabled, next.OverlayHotkey = enabled, key
	return r.svc.Apply(prev, next)
}

func (r *rig) state() (visible bool, prev uintptr) {
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	return r.svc.visible, r.svc.prev
}

func TestToggleShowsOnTheMonitorOfTheForegroundWindow(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()

	if len(r.plat.monitorFor) != 1 || r.plat.monitorFor[0] != gameHwnd {
		t.Fatalf("monitor looked up for %v, want the game window %d", r.plat.monitorFor, gameHwnd)
	}
	if len(r.win.shown) != 1 || r.win.shown[0] != r.plat.area {
		t.Fatalf("window placed at %+v, want %+v", r.win.shown, r.plat.area)
	}
	want := []string{"window:show", "emit:" + EventShown}
	if got := r.log.all(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if !r.win.isVisible() {
		t.Fatal("window is not visible")
	}
}

func TestToggleTwiceReturnsFocusToTheGame(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	r.svc.toggleOnUI()

	want := []string{"window:show", "emit:" + EventShown, "window:hide", "foreground:100", "emit:" + EventHidden}
	if got := r.log.all(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if r.win.isVisible() {
		t.Fatal("window is still visible")
	}
}

func TestShowRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*rig)
		want  []string
	}{
		{"disabled", func(r *rig) { r.svc.cfg.enabled = false }, nil},
		{"no window attached", func(r *rig) { r.svc.win = nil }, nil},
		{"closed", func(r *rig) { r.svc.closed = true }, nil},
		{"monitor lookup fails", func(r *rig) { r.plat.areaErr = errors.New("no monitor") }, nil},
		{"window refuses to show", func(r *rig) { r.win.showErr = errors.New("no handle") }, []string{"window:show", "window:hide"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			c.setup(r)
			r.svc.toggleOnUI()

			if got := r.log.all(); !slices.Equal(got, c.want) {
				t.Fatalf("events = %v, want %v", got, c.want)
			}
			if visible, prev := r.state(); visible || prev != 0 {
				t.Fatalf("state after a refused show: visible=%v prev=%d", visible, prev)
			}
		})
	}
}

func TestHideRestoresFocusOnlyToAValidPreviousWindow(t *testing.T) {
	cases := []struct {
		name      string
		fg        uintptr
		live      bool
		setErr    error
		wantFocus []uintptr
	}{
		{"game window is alive", gameHwnd, true, nil, []uintptr{gameHwnd}},
		{"game window is gone", gameHwnd, false, nil, nil},
		{"nothing was in the foreground", 0, true, nil, nil},
		{"the foreground was the overlay itself", overlayHwnd, true, nil, nil},
		{"windows refuses the focus", gameHwnd, true, errors.New("foreground lock"), []uintptr{gameHwnd}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			r.plat.fg = c.fg
			r.plat.live[gameHwnd] = c.live
			r.svc.toggleOnUI()
			r.plat.foregroundErr = c.setErr
			r.svc.Hide()

			if got := r.plat.foregroundCalls(); !slices.Equal(got, c.wantFocus) {
				t.Fatalf("SetForegroundWindow calls = %v, want %v", got, c.wantFocus)
			}
			if r.win.isVisible() {
				t.Fatal("window is still visible")
			}
			if n := r.log.count("emit:" + EventHidden); n != 1 {
				t.Fatalf("hidden emitted %d times, want 1", n)
			}
		})
	}
}

func TestShowWithoutFocusHidesTheWindow(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.win.noFocus = true
	r.svc.toggleOnUI()

	want := []string{"window:show", "window:hide"}
	if got := r.log.all(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if visible, prev := r.state(); visible || prev != 0 {
		t.Fatalf("state after a show that got no focus: visible=%v prev=%d", visible, prev)
	}
	if r.svc.Visible() {
		t.Fatal("Visible reports an overlay that was taken down")
	}
	if got := r.plat.foregroundCalls(); len(got) != 0 {
		t.Fatalf("SetForegroundWindow calls = %v, the game never lost the foreground", got)
	}
}

func TestHideRestoresAMinimisedGameBeforeFocusingIt(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	r.plat.iconic[gameHwnd] = true
	r.svc.Hide()

	want := []string{"window:show", "emit:" + EventShown, "window:hide", "restore:100", "foreground:100", "emit:" + EventHidden}
	if got := r.log.all(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestHideDoesNotRestoreAWindowThatIsNotMinimised(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.toggleOnUI()
	r.svc.Hide()
	if n := r.log.count("restore:100"); n != 0 {
		t.Fatalf("restored a window that was not minimised %d times", n)
	}
}

func TestWindowIsCreatedOnTheFirstShowOnly(t *testing.T) {
	cases := []struct {
		name      string
		enabled   bool
		toggles   int
		wantMakes int
	}{
		{"never shown", true, 0, 0},
		{"disabled overlay is never created", false, 2, 0},
		{"first show creates it", true, 1, 1},
		{"later shows reuse it", true, 4, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.enabled, settings.OverlayHotkeyAltBacktick)
			r.svc.win = nil
			makes := 0
			r.svc.makeWin = func() window { makes++; return r.win }
			for range c.toggles {
				r.svc.toggleOnUI()
			}
			if makes != c.wantMakes {
				t.Fatalf("window factory called %d times, want %d", makes, c.wantMakes)
			}
		})
	}
}

func TestShownReportsExclusiveFullscreen(t *testing.T) {
	cases := []struct {
		name  string
		state int
		err   error
		want  bool
	}{
		{"exclusive d3d fullscreen", 3, nil, true},
		{"borderless fullscreen window", 2, nil, false},
		{"not present", 1, nil, false},
		{"presentation mode", 4, nil, false},
		{"windows store app", 5, nil, false},
		{"unknown state", 42, nil, false},
		{"query fails", 0, errors.New("no shell"), false},
		{"query fails with an exclusive-looking state", 3, errors.New("no shell"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			r.plat.state, r.plat.stateErr = c.state, c.err
			r.svc.toggleOnUI()

			if got := r.log.shownPayloads(); !slices.Equal(got, []Shown{{Exclusive: c.want}}) {
				t.Fatalf("shown payloads = %v, want one with Exclusive=%v", got, c.want)
			}
			if got := r.svc.View(); got != (View{Visible: true, Exclusive: c.want}) {
				t.Fatalf("View while shown = %+v", got)
			}
			if !r.win.isVisible() {
				t.Fatal("the overlay must be shown whatever the notification state query says")
			}

			r.svc.Hide()
			if got := r.svc.View(); got != (View{}) {
				t.Fatalf("View after hide = %+v, want zero", got)
			}
		})
	}
}

func TestViewIsClearedWhenShowFails(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*rig)
	}{
		{"window refuses to show", func(r *rig) { r.win.showErr = errors.New("no handle") }},
		{"window gets no focus", func(r *rig) { r.win.noFocus = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			r.plat.state = 3
			c.setup(r)
			r.svc.toggleOnUI()
			if got := r.svc.View(); got != (View{}) {
				t.Fatalf("View = %+v after a show that did not happen", got)
			}
			if got := r.log.shownPayloads(); len(got) != 0 {
				t.Fatalf("shown emitted: %v", got)
			}
		})
	}
}

func TestHideWhenNotVisibleDoesNothing(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.svc.Hide()
	if got := r.log.all(); len(got) != 0 {
		t.Fatalf("events = %v, want none", got)
	}
}

func TestLostFocus(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(*rig)
		wantVisible bool
		wantHidden  int
		wantFocus   int
	}{
		{"user switched to another window", func(r *rig) { r.plat.setFg(otherHwnd) }, false, 1, 0},
		{"user switched to the desktop", func(r *rig) { r.plat.setFg(0) }, false, 1, 0},
		{"overlay is still the foreground window", func(*rig) {}, true, 0, 0},
		{"overlay was already hidden by the hotkey", func(r *rig) { r.svc.Hide() }, false, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			r.svc.toggleOnUI()
			c.setup(r)
			r.svc.lostFocus()

			if r.win.isVisible() != c.wantVisible {
				t.Fatalf("visible = %v, want %v", r.win.isVisible(), c.wantVisible)
			}
			if n := r.log.count("emit:" + EventHidden); n != c.wantHidden {
				t.Fatalf("hidden emitted %d times, want %d", n, c.wantHidden)
			}
			if n := len(r.plat.foregroundCalls()); n != c.wantFocus {
				t.Fatalf("SetForegroundWindow called %d times, want %d: a user who left on their own must not be dragged back", n, c.wantFocus)
			}
		})
	}
}

func TestStartup(t *testing.T) {
	cases := []struct {
		name          string
		enabled       bool
		setup         func(*fakePlatform)
		wantActive    []string
		wantError     bool
		wantSupported bool
		wantEmits     int
	}{
		{"enabled", true, func(*fakePlatform) {}, []string{settings.OverlayHotkeyShiftF1}, false, true, 0},
		{"disabled", false, func(*fakePlatform) {}, nil, false, true, 0},
		{"key taken by another program", true, func(p *fakePlatform) {
			p.registerErr[settings.OverlayHotkeyShiftF1] = errors.New("already registered")
		}, nil, true, true, 1},
		{"unsupported platform", true, func(p *fakePlatform) { p.unsupported = true }, nil, false, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.enabled, settings.OverlayHotkeyShiftF1)
			c.setup(r.plat)
			r.start(t)

			if got := r.plat.active(); !slices.Equal(got, c.wantActive) {
				t.Fatalf("registered %v, want %v", got, c.wantActive)
			}
			st := r.svc.Status()
			if (st.Error != "") != c.wantError {
				t.Fatalf("Status.Error = %q, want an error: %v", st.Error, c.wantError)
			}
			if st.Supported != c.wantSupported || st.Enabled != c.enabled || st.Hotkey != settings.OverlayHotkeyShiftF1 {
				t.Fatalf("Status = %+v", st)
			}
			if n := r.log.count("emit:" + EventStatus); n != c.wantEmits {
				t.Fatalf("status emitted %d times, want %d", n, c.wantEmits)
			}
		})
	}
}

func TestApplyChangingTheKeyRegistersTheNewOneBeforeDroppingTheOld(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyShiftF2); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyShiftF2}) {
		t.Fatalf("registered %v, want only Shift+F2", got)
	}
	if st := r.svc.Status(); st.Hotkey != settings.OverlayHotkeyShiftF2 || st.Error != "" {
		t.Fatalf("Status = %+v", st)
	}
	if n := r.log.count("emit:" + EventStatus); n != 1 {
		t.Fatalf("status emitted %d times, want 1", n)
	}
}

func TestApplyKeepsTheOldKeyWhenTheNewOneIsTaken(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	r.plat.registerErr[settings.OverlayHotkeyShiftF1] = errors.New("already registered")

	err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyShiftF1)
	if err == nil {
		t.Fatal("a key that cannot be registered must fail the save")
	}
	if uierr.Code(err) != ErrCodeHotkeyUnavailable {
		t.Fatalf("code = %q, want %q", uierr.Code(err), ErrCodeHotkeyUnavailable)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want the old key to stay", got)
	}
	if st := r.svc.Status(); st.Hotkey != settings.OverlayHotkeyAltBacktick || st.Error != "" {
		t.Fatalf("Status = %+v, a failed save must not change it", st)
	}
	if n := r.log.count("emit:" + EventStatus); n != 0 {
		t.Fatalf("status emitted %d times for a save that did not happen", n)
	}
}

func TestApplyIgnoresUnrelatedSaves(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	before := r.plat.registrations()
	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyAltBacktick); err != nil {
		t.Fatal(err)
	}
	if r.plat.registrations() != before {
		t.Fatal("an unchanged hotkey was registered again")
	}
}

func TestApplyDisableDropsTheKeyAndClosesAVisibleOverlay(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	r.svc.toggleOnUI()

	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, false, settings.OverlayHotkeyAltBacktick); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); len(got) != 0 {
		t.Fatalf("registered %v after disabling", got)
	}
	if r.win.isVisible() {
		t.Fatal("overlay stayed on screen after being disabled")
	}
	if st := r.svc.Status(); st.Enabled {
		t.Fatalf("Status = %+v", st)
	}
	r.svc.toggleOnUI()
	if r.win.isVisible() {
		t.Fatal("a disabled overlay was shown")
	}

	if err := r.apply(false, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyAltBacktick); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v after enabling", got)
	}
}

func TestApplyAfterFailedStartupClearsTheError(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.plat.registerErr[settings.OverlayHotkeyAltBacktick] = errors.New("already registered")
	r.start(t)
	if r.svc.Status().Error == "" {
		t.Fatal("startup failure must be reported in Status")
	}
	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyShiftF2); err != nil {
		t.Fatal(err)
	}
	if st := r.svc.Status(); st.Error != "" || st.Hotkey != settings.OverlayHotkeyShiftF2 {
		t.Fatalf("Status = %+v", st)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyShiftF2}) {
		t.Fatalf("registered %v", got)
	}
}

func TestApplyBeforeStartupIsPickedUpByStartup(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyCtrlShiftO); err != nil {
		t.Fatal(err)
	}
	if r.plat.registrations() != 0 {
		t.Fatal("a key was registered before the service started")
	}
	r.start(t)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyCtrlShiftO}) {
		t.Fatalf("registered %v, want Ctrl+Shift+O", got)
	}
}

func TestApplyRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *rig)
		key   string
		want  error
	}{
		{"unknown key", func(t *testing.T, r *rig) { r.start(t) }, "Alt+Q", ErrUnknownHotkey},
		{"after shutdown", func(t *testing.T, r *rig) {
			r.start(t)
			if err := r.svc.ServiceShutdown(); err != nil {
				t.Fatal(err)
			}
		}, settings.OverlayHotkeyShiftF1, errClosed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
			c.setup(t, r)
			before := r.plat.registrations()
			err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, c.key)
			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if r.plat.registrations() != before {
				t.Fatal("a refused save registered a key")
			}
		})
	}
}

func TestApplyOnAnUnsupportedPlatformOnlyRecordsTheChoice(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.plat.unsupported = true
	r.start(t)
	if err := r.apply(true, settings.OverlayHotkeyAltBacktick, true, settings.OverlayHotkeyShiftF2); err != nil {
		t.Fatal(err)
	}
	if r.plat.registrations() != 0 {
		t.Fatal("a key was registered where the overlay is unsupported")
	}
	if st := r.svc.Status(); st.Supported || st.Hotkey != settings.OverlayHotkeyShiftF2 {
		t.Fatalf("Status = %+v", st)
	}
}

func TestShutdownDropsTheKeyAndIgnoresLatePresses(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	press := r.plat.press
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); len(got) != 0 {
		t.Fatalf("registered %v after shutdown", got)
	}
	press()
	if r.win.isVisible() || len(r.log.all()) != 0 {
		t.Fatalf("a press after shutdown reached the window: %v", r.log.all())
	}
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestPressedHotkeyTogglesTheOverlay(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)
	r.plat.press()
	if !r.win.isVisible() {
		t.Fatal("hotkey did not show the overlay")
	}
	r.plat.press()
	if r.win.isVisible() {
		t.Fatal("second press did not hide the overlay")
	}
}

func TestConcurrentToggleHideAndLostFocus(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	r.start(t)

	const rounds = 300
	var wg sync.WaitGroup
	run := func(n int, f func(i int)) {
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range rounds {
					f(i)
				}
			}()
		}
	}
	run(4, func(int) { r.svc.toggle() })
	run(3, func(int) { r.svc.Hide() })
	run(3, func(i int) {
		if i%2 == 0 {
			r.plat.setFg(otherHwnd)
		}
		r.svc.lostFocus()
	})
	run(1, func(i int) {
		key := settings.OverlayHotkeyShiftF1
		prev := settings.OverlayHotkeyAltBacktick
		if i%2 == 1 {
			key, prev = prev, key
		}
		if err := r.apply(true, prev, true, key); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()

	var edges []string
	for _, e := range r.log.all() {
		if e == "emit:"+EventShown || e == "emit:"+EventHidden {
			edges = append(edges, e)
		}
	}
	for i, e := range edges {
		want := "emit:" + EventShown
		if i%2 == 1 {
			want = "emit:" + EventHidden
		}
		if e != want {
			t.Fatalf("edge %d is %s, want %s: shown and hidden must strictly alternate", i, e, want)
		}
	}
	visible, _ := r.state()
	if visible != (len(edges)%2 == 1) || visible != r.win.isVisible() {
		t.Fatalf("state visible=%v, window visible=%v, %d edges", visible, r.win.isVisible(), len(edges))
	}
	if n := len(r.plat.active()); n != 1 {
		t.Fatalf("%d hotkeys registered after the storm, want 1", n)
	}
}
