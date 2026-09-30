package overlay

import (
	"context"
	"sync"
	"testing"
)

// Ctrl+Alt+Shift+F12 is not one of the four keys the launcher offers, so a
// running launcher on the same machine cannot be holding it.
var testKey = hotkey{mods: modCtrl | modAlt | modShift, vk: 0x7B}

func TestRegisterRefusesAKeyThatIsAlreadyTaken(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := windowsPlatform{}

	stopFirst, err := p.register(ctx, &wg, testKey, func() {})
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if _, err := p.register(ctx, &wg, testKey, func() {}); err == nil {
		t.Fatal("the same key was registered twice")
	}
	stopFirst()

	stopAgain, err := p.register(ctx, &wg, testKey, func() {})
	if err != nil {
		t.Fatalf("registration after the key was released: %v", err)
	}
	stopAgain()
	stopAgain()
	cancel()
	wg.Wait()
}

func TestRegisterStopsWhenTheContextIsCancelled(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	p := windowsPlatform{}

	if _, err := p.register(ctx, &wg, testKey, func() {}); err != nil {
		t.Fatalf("registration: %v", err)
	}
	cancel()
	wg.Wait()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	stop, err := p.register(ctx2, &wg, testKey, func() {})
	if err != nil {
		t.Fatalf("the key stayed registered after the context was cancelled: %v", err)
	}
	stop()
	cancel2()
	wg.Wait()
}

func TestMonitorRectOfTheForegroundWindow(t *testing.T) {
	p := windowsPlatform{}
	for _, hwnd := range []uintptr{p.foreground(), 0} {
		area, err := p.monitorRect(hwnd)
		if err != nil {
			t.Fatalf("monitorRect(%d): %v", hwnd, err)
		}
		if area.w <= 0 || area.h <= 0 {
			t.Fatalf("monitorRect(%d) = %+v", hwnd, area)
		}
	}
}

func TestIsWindowRejectsAStaleHandle(t *testing.T) {
	if (windowsPlatform{}).isWindow(0xFFFFFF0) {
		t.Fatal("a handle that names no window was accepted")
	}
}

func TestNotificationStateIsAKnownValue(t *testing.T) {
	state, err := (windowsPlatform{}).notificationState()
	if err != nil {
		t.Fatalf("notificationState: %v", err)
	}
	if state < 1 || state > 7 {
		t.Fatalf("state = %d, want one of QUNS_NOT_PRESENT..QUNS_APP (1-7)", state)
	}
}
