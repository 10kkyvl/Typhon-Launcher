package main

import (
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"typhon/internal/dialogtext"
	"typhon/internal/jumplist"
	"typhon/internal/library"
	"typhon/internal/shortcut"
	"typhon/internal/tray"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const recentRefreshDelay = 500 * time.Millisecond

// trayPlayer is what the Play button in the UI calls: it honours the last
// launch profile of the game, unlike --play, which names its profile itself.
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

// recentMenu owns the part of the tray menu that follows the library. Wails
// builds a native menu once, so a change is a whole new menu handed to SetMenu;
// that is never done while the menu is on screen, because destroying a popup
// that is being tracked closes it under the user's cursor.
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
	stale      atomic.Bool
	systemTray atomic.Pointer[application.SystemTray]

	mu    sync.Mutex
	games []tray.Game
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

// refresh runs on the coalescer's goroutine, outside every library lock.
func (m *recentMenu) refresh() {
	m.setGames(m.fetch())
	if !m.running.Load() {
		return
	}
	systemTray := m.systemTray.Load()
	if systemTray == nil {
		return
	}
	application.InvokeSync(func() {
		if m.showing.Load() {
			m.stale.Store(true)
			return
		}
		systemTray.SetMenu(m.build())
	})
}

func (m *recentMenu) show(systemTray *application.SystemTray) {
	application.InvokeSync(func() {
		m.showing.Store(true)
		defer m.showing.Store(false)
		systemTray.SetMenu(m.build())
		systemTray.ShowMenu()
		m.showing.Store(false)
		if m.stale.CompareAndSwap(true, false) {
			systemTray.SetMenu(m.build())
		}
	})
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
	systemTray.SetMenu(menu.build())
	systemTray.OnClick(menu.open)
	systemTray.OnRightClick(func() { menu.show(systemTray) })
	menu.systemTray.Store(systemTray)
	return trayHandle{SystemTray: systemTray, menu: menu}, nil
}

func trayGames(recent []library.RecentGame) []tray.Game {
	games := make([]tray.Game, len(recent))
	for i, game := range recent {
		games[i] = tray.Game{ID: game.ID, Title: game.Title, Running: game.Running}
	}
	return games
}

// jumpListRefresher is nil where there is no Jump List or the launcher path
// cannot be resolved: the list is a convenience, and the second case is logged
// once here instead of on every refresh.
func jumpListRefresher(recent *library.Service, language func() string) func() {
	if !jumplist.Supported() {
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
