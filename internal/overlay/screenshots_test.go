package overlay

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"typhon/internal/settings"
)

func TestParseScreenshotHotkey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want hotkey
		err  error
	}{
		{"f12", "F12", hotkey{vk: 0x7B}, nil},
		{"f11", "F11", hotkey{vk: 0x7A}, nil},
		{"shift f12", "Shift+F12", hotkey{mods: modShift, vk: 0x7B}, nil},
		{"ctrl f12", "Ctrl+F12", hotkey{mods: modCtrl, vk: 0x7B}, nil},
		{"empty", "", hotkey{}, ErrUnknownHotkey},
		{"overlay key", "Alt+`", hotkey{}, ErrUnknownHotkey},
		{"lower case", "f12", hotkey{}, ErrUnknownHotkey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseScreenshotHotkey(c.in)
			if !errors.Is(err, c.err) {
				t.Fatalf("error = %v, want %v", err, c.err)
			}
			if got != c.want {
				t.Fatalf("hotkey = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestEverySettingsScreenshotHotkeyParses(t *testing.T) {
	for _, name := range settings.ScreenshotHotkeys() {
		if _, err := parseScreenshotHotkey(name); err != nil {
			t.Errorf("%q is accepted by settings but not by the overlay: %v", name, err)
		}
	}
	if len(screenshotHotkeys) != len(settings.ScreenshotHotkeys()) {
		t.Errorf("overlay knows %d screenshot hotkeys, settings allows %d", len(screenshotHotkeys), len(settings.ScreenshotHotkeys()))
	}
}

func shotRig(t *testing.T, shots bool, key string) *rig {
	t.Helper()
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.svc.ConfigureScreenshots(shots, key); err != nil {
		t.Fatal(err)
	}
	r.svc.SetScreenshotsArmed(true)
	return r
}

func TestStartupRegistersBothKeys(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.start(t)
	want := []string{settings.OverlayHotkeyAltBacktick, settings.ScreenshotHotkeyF12}
	if got := r.plat.active(); !slices.Equal(got, want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
}

func TestStartupSkipsScreenshotKeyWhenDisabled(t *testing.T) {
	r := shotRig(t, false, settings.ScreenshotHotkeyF12)
	r.start(t)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want only the overlay key", got)
	}
}

func TestScreenshotKeyTakenDoesNotBreakTheOverlayKey(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.plat.registerErr[settings.ScreenshotHotkeyF12] = errors.New("already registered")
	r.start(t)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want only the overlay key", got)
	}
	st := r.svc.Status()
	if st.Error == "" {
		t.Fatalf("the failed screenshot key is not in Status: %+v", st)
	}
	if n := r.log.count("emit:" + EventStatus); n != 1 {
		t.Fatalf("status emitted %d times, want 1", n)
	}
}

func TestOverlayKeyTakenDoesNotBreakTheScreenshotKey(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.plat.registerErr[settings.OverlayHotkeyAltBacktick] = errors.New("already registered")
	r.start(t)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.ScreenshotHotkeyF12}) {
		t.Fatalf("registered %v, want only the screenshot key", got)
	}
	if r.svc.Status().Error == "" {
		t.Fatal("the failed overlay key is not in Status")
	}
}

func applyShots(r *rig, prevOn bool, prevKey string, on bool, key string) error {
	prev, next := settings.Defaults(), settings.Defaults()
	prev.ScreenshotsEnabled, prev.ScreenshotHotkey = prevOn, prevKey
	next.ScreenshotsEnabled, next.ScreenshotHotkey = on, key
	return r.svc.Apply(prev, next)
}

func TestApplyTogglesScreenshotKey(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.start(t)

	if err := applyShots(r, true, settings.ScreenshotHotkeyF12, false, settings.ScreenshotHotkeyF12); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v after disabling", got)
	}

	if err := applyShots(r, false, settings.ScreenshotHotkeyF12, true, settings.ScreenshotHotkeyShiftF12); err != nil {
		t.Fatal(err)
	}
	want := []string{settings.OverlayHotkeyAltBacktick, settings.ScreenshotHotkeyShiftF12}
	if got := r.plat.active(); !slices.Equal(got, want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
}

func TestApplyKeepsOldScreenshotKeyWhenNewIsTaken(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.start(t)
	r.plat.registerErr[settings.ScreenshotHotkeyF11] = errors.New("already registered")
	if err := applyShots(r, true, settings.ScreenshotHotkeyF12, true, settings.ScreenshotHotkeyF11); err == nil {
		t.Fatal("a taken key must be reported")
	}
	want := []string{settings.OverlayHotkeyAltBacktick, settings.ScreenshotHotkeyF12}
	if got := r.plat.active(); !slices.Equal(got, want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
}

func TestScreenshotPressHandsTheForegroundWindowToTheHandler(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	var got []uintptr
	r.svc.SetScreenshotHandler(func(hwnd uintptr) { got = append(got, hwnd) })
	r.start(t)
	r.plat.shotPress()
	if len(got) != 1 || got[0] != gameHwnd {
		t.Fatalf("handler got %v, want the game window %d", got, gameHwnd)
	}
}

func TestScreenshotPressWithoutHandlerIsIgnored(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.start(t)
	r.plat.shotPress()
}

func TestConfigureScreenshotsRejectsUnknownKey(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.svc.ConfigureScreenshots(true, "F10"); !errors.Is(err, ErrUnknownHotkey) {
		t.Fatalf("error = %v, want ErrUnknownHotkey", err)
	}
}

func TestScreenshotKeyIsNotRegisteredWhileNoGameRuns(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.svc.ConfigureScreenshots(true, settings.ScreenshotHotkeyF12); err != nil {
		t.Fatal(err)
	}
	r.start(t)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want only the overlay key", got)
	}

	r.svc.SetScreenshotsArmed(true)
	want := []string{settings.OverlayHotkeyAltBacktick, settings.ScreenshotHotkeyF12}
	if got := r.plat.active(); !slices.Equal(got, want) {
		t.Fatalf("registered %v after arming, want %v", got, want)
	}
	r.svc.SetScreenshotsArmed(true)
	if r.plat.registrations() != 2 {
		t.Fatalf("arming twice registered the key again: %d registrations", r.plat.registrations())
	}

	r.svc.SetScreenshotsArmed(false)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v after disarming", got)
	}
}

func TestArmingWithScreenshotsDisabledRegistersNothing(t *testing.T) {
	r := shotRig(t, false, settings.ScreenshotHotkeyF12)
	r.start(t)
	r.svc.SetScreenshotsArmed(true)
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want only the overlay key", got)
	}
}

func TestApplyWhileNotArmedKeepsTheKeyReleased(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.svc.ConfigureScreenshots(true, settings.ScreenshotHotkeyF12); err != nil {
		t.Fatal(err)
	}
	r.start(t)
	if err := applyShots(r, true, settings.ScreenshotHotkeyF12, true, settings.ScreenshotHotkeyF11); err != nil {
		t.Fatal(err)
	}
	if got := r.plat.active(); !slices.Equal(got, []string{settings.OverlayHotkeyAltBacktick}) {
		t.Fatalf("registered %v, want only the overlay key", got)
	}
	r.svc.SetScreenshotsArmed(true)
	want := []string{settings.OverlayHotkeyAltBacktick, settings.ScreenshotHotkeyF11}
	if got := r.plat.active(); !slices.Equal(got, want) {
		t.Fatalf("registered %v, want the new key %v", got, want)
	}
}

func TestPressWhileNotArmedIsIgnored(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	called := false
	r.svc.SetScreenshotHandler(func(uintptr) { called = true })
	r.start(t)
	r.svc.SetScreenshotsArmed(false)
	r.svc.shotPressed()
	if called {
		t.Fatal("the handler ran while no game was running")
	}
}

func TestArmedFailureIsReportedInStatus(t *testing.T) {
	r := newRig(t, true, settings.OverlayHotkeyAltBacktick)
	if err := r.svc.ConfigureScreenshots(true, settings.ScreenshotHotkeyF12); err != nil {
		t.Fatal(err)
	}
	r.plat.registerErr[settings.ScreenshotHotkeyF12] = errors.New("already registered")
	r.start(t)
	r.svc.SetScreenshotsArmed(true)
	if r.svc.Status().Error == "" {
		t.Fatal("the failed registration is not in Status")
	}
	delete(r.plat.registerErr, settings.ScreenshotHotkeyF12)
	r.svc.SetScreenshotsArmed(false)
	r.svc.SetScreenshotsArmed(true)
	if st := r.svc.Status(); st.Error != "" {
		t.Fatalf("Status.Error = %q after a successful registration", st.Error)
	}
}

func TestScreenshotReconfigureRacesCleanly(t *testing.T) {
	r := shotRig(t, true, settings.ScreenshotHotkeyF12)
	r.svc.SetScreenshotHandler(func(uintptr) {})
	r.start(t)

	keys := settings.ScreenshotHotkeys()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 50 {
			prev, next := settings.Defaults(), settings.Defaults()
			prev.ScreenshotHotkey = keys[i%len(keys)]
			next.ScreenshotHotkey = keys[(i+1)%len(keys)]
			next.ScreenshotsEnabled = i%7 != 0
			if err := r.svc.Apply(prev, next); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			r.svc.shotPressed()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 50 {
			r.svc.SetScreenshotsArmed(i%2 == 0)
		}
	}()
	wg.Wait()
}
