package overlay

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//wails:ignore
func (s *Service) Attach(app *application.App) {
	if !s.plat.supported() {
		return
	}
	s.mu.Lock()
	s.makeWin = func() window {
		w, native := newWindow(app)
		// A destroyed window is never created again, so closing it (Alt+F4) has to
		// hide it instead.
		native.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			e.Cancel()
			s.Hide()
		})
		native.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { s.lostFocus() })
		return w
	}
	s.makeBrowser = func() (browserWindow, error) {
		owner := func() uintptr {
			s.mu.Lock()
			win := s.win
			s.mu.Unlock()
			if win == nil {
				return 0
			}
			return win.handle()
		}
		b, native := newBrowserWindow(app, owner, s.Hide)
		native.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			e.Cancel()
			s.dispatch(s.closeBrowserOnUI)
		})
		native.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { s.lostFocus() })
		return b, nil
	}
	s.mu.Unlock()
	app.Event.On(EventHide, func(*application.CustomEvent) { s.Hide() })
}
