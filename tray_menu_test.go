package main

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeTrayPlayer struct {
	played []string
	err    error
}

func (p *fakeTrayPlayer) PlayGame(id string) error {
	p.played = append(p.played, id)
	return p.err
}

type fakeTrayStopper struct {
	stopped []string
	err     error
}

func (s *fakeTrayStopper) StopGame(id string) error {
	s.stopped = append(s.stopped, id)
	return s.err
}

func TestPlayFromTray(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReveal int
	}{
		{"launched game leaves the window alone", nil, 0},
		{"a refused launch opens the window", errors.New("already running"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			player := &fakeTrayPlayer{err: tc.err}
			revealed := 0
			playFromTray(player, "g1", func() { revealed++ })
			if len(player.played) != 1 || player.played[0] != "g1" {
				t.Fatalf("PlayGame calls = %v, want [g1]", player.played)
			}
			if revealed != tc.wantReveal {
				t.Fatalf("window opened %d times, want %d", revealed, tc.wantReveal)
			}
		})
	}
}

func TestStopFromTray(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReveal int
	}{
		{"stopped game leaves the window alone", nil, 0},
		{"a failed stop opens the window", errors.New("process gone"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stopper := &fakeTrayStopper{err: tc.err}
			revealed := 0
			stopFromTray(stopper, "g1", func() { revealed++ })
			if len(stopper.stopped) != 1 || stopper.stopped[0] != "g1" {
				t.Fatalf("StopGame calls = %v, want [g1]", stopper.stopped)
			}
			if revealed != tc.wantReveal {
				t.Fatalf("window opened %d times, want %d", revealed, tc.wantReveal)
			}
		})
	}
}

func newTestMenu() *recentMenu {
	m := &recentMenu{language: func() string { return "en" }}
	m.running.Store(true)
	return m
}

func TestReplaceIfIdle(t *testing.T) {
	current := &application.SystemTray{}
	other := &application.SystemTray{}
	cases := []struct {
		name    string
		running bool
		showing bool
		tray    *application.SystemTray
		want    bool
	}{
		{"idle menu is replaced", true, false, current, true},
		{"shutdown has begun", false, false, current, false},
		{"a popup is on screen", true, true, current, false},
		{"the tray was swapped meanwhile", true, false, other, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMenu()
			m.systemTray.Store(current)
			m.running.Store(tc.running)
			m.showing.Store(tc.showing)
			sets := 0
			got := m.replaceIfIdle(tc.tray, func(*application.Menu) { sets++ })
			if got != tc.want || (sets == 1) != tc.want {
				t.Fatalf("replaceIfIdle = %v with %d installs, want %v", got, sets, tc.want)
			}
		})
	}
}

func TestPresentNeverReplacesTheMenuAfterDisplay(t *testing.T) {
	m := newTestMenu()
	tray := &application.SystemTray{}
	m.systemTray.Store(tray)
	var order []string
	set := func(*application.Menu) { order = append(order, "set") }
	m.present(set, func() {
		order = append(order, "display")
		if !m.showing.Load() {
			t.Error("showing is false while the menu is displayed")
		}
		if m.replaceIfIdle(tray, set) {
			t.Error("a refresh replaced the menu that is on screen")
		}
	})
	if m.showing.Load() {
		t.Fatal("showing stays true after display returned")
	}
	if got := len(order); got != 2 || order[0] != "set" || order[1] != "display" {
		t.Fatalf("calls = %v, want [set display] and nothing after display", order)
	}
}

func TestSwapFreesTheOldMenu(t *testing.T) {
	m := newTestMenu()
	first := m.build()
	m.swap(first, func(*application.Menu) {})
	second := m.build()
	m.swap(second, func(*application.Menu) {})
	if first.ItemAt(0) != nil {
		t.Fatal("the replaced menu keeps its items registered")
	}
	if second.ItemAt(0) == nil {
		t.Fatal("the installed menu was destroyed")
	}
}

func TestJumpListAllowed(t *testing.T) {
	cases := []struct {
		name     string
		portable bool
		qa       bool
		want     bool
	}{
		{"installed launcher", false, false, true},
		{"portable copy", true, false, false},
		{"qa build", false, true, false},
		{"portable qa build", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jumpListAllowed(tc.portable, tc.qa); got != tc.want {
				t.Fatalf("jumpListAllowed(%v, %v) = %v, want %v", tc.portable, tc.qa, got, tc.want)
			}
		})
	}
}

func TestJumpListRefresherIsOffWhenNotAllowed(t *testing.T) {
	if refresh := jumpListRefresher(nil, func() string { return "en" }, false); refresh != nil {
		t.Fatal("a refresher was built for a launcher that must not touch the shared list")
	}
}

func TestWaitBounded(t *testing.T) {
	t.Run("returns when the wait finishes", func(t *testing.T) {
		waitBounded("fast", time.Minute, func() {})
	})
	t.Run("gives up on a wait that never ends", func(t *testing.T) {
		stuck := make(chan struct{})
		t.Cleanup(func() { close(stuck) })
		waitBounded("stuck", 10*time.Millisecond, func() { <-stuck })
	})
}

func TestKeepTrayMenuStoresTheMenuOpenMenuChecks(t *testing.T) {
	systemTray := new(application.SystemTray)
	menu := application.NewMenu()
	if err := keepTrayMenu(systemTray, menu); err != nil {
		t.Fatalf("keepTrayMenu: %v", err)
	}
	got := reflect.ValueOf(systemTray).Elem().FieldByName("menu")
	if got.Pointer() != reflect.ValueOf(menu).Pointer() {
		t.Fatalf("the tray keeps %v, want the menu it was given", got)
	}
}
