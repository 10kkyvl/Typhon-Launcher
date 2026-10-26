package main

import (
	"errors"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"typhon/internal/dialogtext"
	"typhon/internal/jumplist"
	"typhon/internal/library"
	"typhon/internal/shortcut"
	"typhon/internal/tray"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	recentRefreshDelay = 500 * time.Millisecond
	shutdownWaitLimit  = 3 * time.Second
)

// PlayGame, not PlayGameProfile: the tray acts like the Play button and keeps
// the last launch profile of the game.
type trayPlayer interface {
	PlayGame(id string) error
}

type trayStopper interface {
	StopGame(id string) error
}

func playFromTray(player trayPlayer, id string, reveal func()) {
	if err := player.PlayGame(id); err != nil {
		slog.Error("play from tray", "id", id, "error", err)
		reveal()
	}
}

func stopFromTray(stopper trayStopper, id string, reveal func()) {
	if err := stopper.StopGame(id); err != nil {
		slog.Error("stop from tray", "id", id, "error", err)
		reveal()
	}
}

type recentMenu struct {
	fetch    func() []tray.Game
	language func() string
	open     func()
	quit     func()
	play     func(id string)
	stop     func(id string)

	async      tray.Async
	running    atomic.Bool
	showing    atomic.Bool
	systemTray atomic.Pointer[application.SystemTray]

	mu      sync.Mutex
	games   []tray.Game
	current *application.Menu
}

func (m *recentMenu) setGames(games []tray.Game) {
	m.mu.Lock()
	m.games = games
	m.mu.Unlock()
}

func (m *recentMenu) snapshot() []tray.Game {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.games
}

func (m *recentMenu) refresh() {
	m.setGames(m.fetch())
	systemTray := m.systemTray.Load()
	if systemTray == nil || !m.running.Load() {
		return
	}
	application.InvokeSync(func() {
		m.replaceIfIdle(systemTray, func(menu *application.Menu) { systemTray.SetMenu(menu) })
	})
}

// Replacing a menu while its popup is tracked closes it under the cursor; the
// next right click rebuilds the menu anyway.
func (m *recentMenu) replaceIfIdle(systemTray *application.SystemTray, set func(*application.Menu)) bool {
	if !m.running.Load() || m.showing.Load() || m.systemTray.Load() != systemTray {
		return false
	}
	m.swap(m.build(), set)
	return true
}

// swap frees the old menu only after the new one is installed: Wails keeps
// every item in a global map until it is destroyed.
func (m *recentMenu) swap(menu *application.Menu, set func(*application.Menu)) {
	set(menu)
	m.mu.Lock()
	old := m.current
	m.current = menu
	m.mu.Unlock()
	if old != nil {
		old.Destroy()
	}
}

func (m *recentMenu) show(systemTray *application.SystemTray) {
	application.InvokeSync(func() {
		m.present(func(menu *application.Menu) { setTrayMenu(systemTray, menu) }, systemTray.ShowMenu)
	})
}

// The menu is never replaced after display returns: TrackPopupMenuEx returns
// before the click is dispatched, and Wails resolves the item by position in
// the menu installed at that moment, so a new menu would send the click to a
// different item.
func (m *recentMenu) present(set func(*application.Menu), display func()) {
	m.showing.Store(true)
	defer m.showing.Store(false)
	m.swap(m.build(), set)
	display()
}

func (m *recentMenu) build() *application.Menu {
	labels := dialogtext.For(m.language())
	items := tray.BuildItems(tray.Labels{
		Recent:  labels.RecentGames,
		Playing: labels.Playing,
		Stop:    labels.StopGame,
		Open:    labels.OpenTyphon,
		Quit:    labels.Quit,
	}, m.snapshot())
	menu := application.NewMenu()
	for _, item := range items {
		if item.Kind == tray.KindSeparator {
			menu.AddSeparator()
			continue
		}
		entry := menu.Add(tray.NativeLabel(item.Label, runtime.GOOS == "windows"))
		switch item.Kind {
		case tray.KindHeader:
			entry.SetEnabled(false)
		case tray.KindGame:
			id := item.GameID
			entry.SetEnabled(!item.Disabled)
			entry.OnClick(func(*application.Context) { m.async.Go(func() { m.play(id) }) })
		case tray.KindStop:
			id := item.GameID
			entry.OnClick(func(*application.Context) { m.async.Go(func() { m.stop(id) }) })
		case tray.KindOpen:
			entry.OnClick(func(*application.Context) { m.open() })
		case tray.KindQuit:
			entry.OnClick(func(*application.Context) { m.quit() })
		}
	}
	return menu
}

type trayHandle struct {
	*application.SystemTray
	menu *recentMenu
}

func (h trayHandle) Destroy() {
	h.menu.systemTray.CompareAndSwap(h.SystemTray, nil)
	h.SystemTray.Destroy()
}

func newSystemTray(wails *application.App, menu *recentMenu) (tray.Tray, error) {
	systemTray := wails.SystemTray.New()
	systemTray.SetIcon(trayIcon)
	systemTray.SetTooltip("Typhon")
	menu.swap(menu.build(), func(built *application.Menu) { setTrayMenu(systemTray, built) })
	systemTray.OnClick(menu.open)
	systemTray.OnRightClick(func() { menu.show(systemTray) })
	menu.systemTray.Store(systemTray)
	return trayHandle{SystemTray: systemTray, menu: menu}, nil
}

var errTrayMenuField = errors.New("wails SystemTray has no menu field to keep")

func setTrayMenu(systemTray *application.SystemTray, menu *application.Menu) {
	systemTray.SetMenu(menu)
	if err := keepTrayMenu(systemTray, menu); err != nil {
		slog.Error("keep tray menu", "error", err)
	}
}

// Wails v3 beta.17 stores the menu that SystemTray.OpenMenu checks only when
// SetMenu runs before the tray starts. A tray turned on in the settings starts
// at once, so its SetMenu reaches the native menu alone and every right click
// found no menu and opened nothing.
func keepTrayMenu(systemTray *application.SystemTray, menu *application.Menu) error {
	field := reflect.ValueOf(systemTray).Elem().FieldByName("menu")
	if !field.IsValid() || field.Type() != reflect.TypeOf(menu) {
		return errTrayMenuField
	}
	//nolint:gosec // G103: the address is of a live field of the tray, written once with a value of its own type.
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(menu))
	return nil
}

func trayGames(recent []library.RecentGame) []tray.Game {
	games := make([]tray.Game, len(recent))
	for i, game := range recent {
		games[i] = tray.Game{ID: game.ID, Title: game.Title, Running: game.Running}
	}
	return games
}

// The Jump List is keyed by the shared AppUserModelID: a portable copy or a qa
// build would overwrite the list of the installed launcher with its own exe.
func jumpListAllowed(portable, qa bool) bool {
	return !portable && !qa
}

// jumpListRefresher is nil where the list must not be touched or the launcher
// path cannot be resolved; the second case is logged once here.
func jumpListRefresher(recent *library.Service, language func() string, allowed bool) func() {
	if !allowed || !jumplist.Supported() {
		return nil
	}
	launcher, err := os.Executable()
	if err != nil {
		slog.Error("resolve launcher for jump list", "component", "app", "operation", "jumplist", "error", err)
		return nil
	}
	return func() {
		labels := dialogtext.For(language())
		games := recent.RecentGames(tray.MaxRecent)
		list := make([]jumplist.Game, len(games))
		for i, game := range games {
			list[i] = jumplist.Game{ID: game.ID, Title: game.Title, Executable: game.Executable}
		}
		cfg := jumplist.Config{
			AppID:    shortcut.LauncherAppID,
			Launcher: launcher,
			Labels:   jumplist.Labels{Recent: labels.RecentGames, Library: labels.LibraryTask, Downloads: labels.DownloadsTask},
		}
		if err := jumplist.Apply(cfg, list); err != nil {
			slog.Error("update jump list", "component", "app", "operation", "jumplist", "error", err)
		}
	}
}

// waitBounded exists for exit only: a COM call stuck inside explorer.exe must
// not keep the launcher from quitting, so the wait is abandoned after limit.
func waitBounded(name string, limit time.Duration, wait func()) {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		slog.Warn("shutdown wait abandoned", "what", name, "limit", limit)
	}
}
