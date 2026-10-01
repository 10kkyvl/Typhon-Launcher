package overlay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const (
	EventShown  = "overlay:shown"
	EventHidden = "overlay:hidden"
	EventStatus = "overlay:status"
	EventHide   = "overlay:hide"

	WindowName = "overlay"
	WindowURL  = "/?overlay=1"

	ErrCodeHotkeyUnavailable = "overlay.hotkey_unavailable"
)

var errClosed = errors.New("overlay service is shut down")

type Status struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Hotkey    string `json:"hotkey"`
	Error     string `json:"error"`
}

type Signal struct{}

const qunsRunningD3DFullScreen = 3

type rect struct {
	x, y, w, h int32
}

type platform interface {
	supported() bool
	register(ctx context.Context, wg *sync.WaitGroup, key hotkey, onPress func()) (stop func(), err error)
	foreground() uintptr
	monitorRect(hwnd uintptr) (rect, error)
	setForeground(hwnd uintptr) error
	isWindow(hwnd uintptr) bool
	exclusiveOwnership() (bool, error)
	notificationState() (int, error)
	isIconic(hwnd uintptr) bool
	restore(hwnd uintptr)
}

type window interface {
	handle() uintptr
	show(r rect) error
	hide()
}

type registration struct {
	name string
	stop func()
}

type config struct {
	enabled bool
	hotkey  string
}

type Service struct {
	plat     platform
	dispatch func(func())
	emit     func(name string, payload any)

	regMu sync.Mutex
	reg   *registration

	mu      sync.Mutex
	ctx     context.Context
	wg      sync.WaitGroup
	cfg     config
	errText string
	win     window
	makeWin func() window
	visible bool
	prev    uintptr
	closed  bool
}

//wails:ignore
func NewService(enabled bool, hotkeyName string) (*Service, error) {
	return newService(newPlatform(), application.InvokeAsync, emitToApp, enabled, hotkeyName)
}

func emitToApp(name string, payload any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, payload)
	}
}

func newService(plat platform, dispatch func(func()), emit func(string, any), enabled bool, hotkeyName string) (*Service, error) {
	if _, err := parseHotkey(hotkeyName); err != nil {
		return nil, err
	}
	return &Service{plat: plat, dispatch: dispatch, emit: emit, cfg: config{enabled: enabled, hotkey: hotkeyName}}, nil
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	return Status{Supported: s.plat.supported(), Enabled: s.cfg.enabled, Hotkey: s.cfg.hotkey, Error: s.errText}
}

func (s *Service) emitStatus() {
	s.emit(EventStatus, s.Status())
}

//wails:ignore
func (s *Service) Visible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visible
}

func (s *Service) Hide() {
	s.dispatch(func() { s.hideOnUI(true) })
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.regMu.Lock()
	s.mu.Lock()
	s.ctx = ctx
	cfg := s.cfg
	s.mu.Unlock()
	if !s.plat.supported() || !cfg.enabled {
		s.regMu.Unlock()
		return nil
	}
	reg, err := s.registerLocked(cfg.hotkey)
	if err == nil {
		s.reg = reg
	}
	s.regMu.Unlock()
	if err != nil {
		slog.Warn("register overlay hotkey", "hotkey", cfg.hotkey, "error", err)
		s.mu.Lock()
		s.errText = err.Error()
		s.mu.Unlock()
		s.emitStatus()
	}
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.regMu.Lock()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	reg := s.reg
	s.reg = nil
	s.regMu.Unlock()
	if reg != nil {
		reg.stop()
	}
	s.wg.Wait()
	return nil
}

//wails:ignore
func (s *Service) Apply(prev, next settings.Settings) error {
	if prev.OverlayEnabled == next.OverlayEnabled && prev.OverlayHotkey == next.OverlayHotkey {
		return nil
	}
	return s.reconfigure(config{enabled: next.OverlayEnabled, hotkey: next.OverlayHotkey})
}

func (s *Service) reconfigure(next config) error {
	if _, err := parseHotkey(next.hotkey); err != nil {
		return err
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errClosed
	}
	started := s.ctx != nil
	s.mu.Unlock()

	if !s.plat.supported() || !started {
		s.setConfig(next, "")
		return nil
	}

	if !next.enabled {
		old := s.reg
		s.reg = nil
		if old != nil {
			old.stop()
		}
		s.setConfig(next, "")
		s.dispatch(func() { s.hideOnUI(true) })
		s.emitStatus()
		return nil
	}

	if s.reg != nil && s.reg.name == next.hotkey {
		s.setConfig(next, "")
		s.emitStatus()
		return nil
	}

	fresh, err := s.registerLocked(next.hotkey)
	if err != nil {
		return err
	}
	old := s.reg
	s.reg = fresh
	if old != nil {
		old.stop()
	}
	s.setConfig(next, "")
	s.emitStatus()
	return nil
}

func (s *Service) setConfig(cfg config, errText string) {
	s.mu.Lock()
	s.cfg = cfg
	s.errText = errText
	s.mu.Unlock()
}

func (s *Service) registerLocked(name string) (*registration, error) {
	key, err := parseHotkey(name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	ctx := s.ctx
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, errClosed
	}
	stop, err := s.plat.register(ctx, &s.wg, key, s.toggle)
	if err != nil {
		return nil, uierr.Wrap(ErrCodeHotkeyUnavailable, fmt.Errorf("register %s: %w", name, err))
	}
	return &registration{name: name, stop: stop}, nil
}

func (s *Service) toggle() {
	s.dispatch(s.toggleOnUI)
}

func (s *Service) lostFocus() {
	s.dispatch(s.lostFocusOnUI)
}

func (s *Service) toggleOnUI() {
	s.mu.Lock()
	visible := s.visible
	usable := s.cfg.enabled && !s.closed && (s.win != nil || s.makeWin != nil)
	s.mu.Unlock()
	if visible {
		s.hideOnUI(true)
		return
	}
	if usable {
		s.showOnUI()
	}
}

// Taking the foreground from a game in exclusive fullscreen makes it minimise
// behind a black mode switch, so the overlay stays closed then. Both probes run
// before the window exists or focus moves, since either changes their answer.
func (s *Service) exclusiveFullscreen() (blocked, own bool, state int) {
	own, ownErr := s.plat.exclusiveOwnership()
	if ownErr != nil {
		slog.Warn("query exclusive ownership", "error", ownErr)
	}
	state, stateErr := s.plat.notificationState()
	if stateErr != nil {
		slog.Warn("query notification state", "error", stateErr)
	}
	// With no answer from Windows there is no way to know the mode, and an overlay that never opens is worse than the rare risk.
	return own || state == qunsRunningD3DFullScreen, own, state
}

func (s *Service) showOnUI() {
	blocked, ownership, state := s.exclusiveFullscreen()
	if blocked {
		slog.Info("overlay skipped: exclusive fullscreen", "exclusive_ownership", ownership, "notification_state", state)
		return
	}
	s.mu.Lock()
	win, create := s.win, s.makeWin
	s.mu.Unlock()
	if win == nil {
		if create == nil {
			return
		}
		win = create()
		s.mu.Lock()
		s.win = win
		s.mu.Unlock()
	}
	own := win.handle()
	prev := s.plat.foreground()
	if prev == own {
		prev = 0
	}
	area, err := s.plat.monitorRect(prev)
	if err != nil {
		slog.Warn("find monitor for overlay", "error", err)
		return
	}
	s.mu.Lock()
	s.visible = true
	s.prev = prev
	s.mu.Unlock()
	if err := win.show(area); err != nil {
		slog.Warn("show overlay window", "error", err)
		s.mu.Lock()
		s.visible = false
		s.prev = 0
		s.mu.Unlock()
		win.hide()
		return
	}
	if fg := s.plat.foreground(); fg != win.handle() {
		slog.Warn("overlay window did not take focus, hiding it", "foreground", fg)
		s.mu.Lock()
		s.visible = false
		s.prev = 0
		s.mu.Unlock()
		win.hide()
		return
	}
	slog.Info("overlay shown", "exclusive_ownership", ownership, "notification_state", state)
	s.emit(EventShown, Signal{})
}

func (s *Service) hideOnUI(restore bool) {
	s.mu.Lock()
	if !s.visible {
		s.mu.Unlock()
		return
	}
	prev := s.prev
	win := s.win
	s.visible = false
	s.prev = 0
	s.mu.Unlock()
	own := win.handle()
	win.hide()
	if restore && prev != 0 && prev != own {
		s.restoreForeground(prev)
	}
	s.emit(EventHidden, Signal{})
}

func (s *Service) restoreForeground(prev uintptr) {
	if !s.plat.isWindow(prev) {
		return
	}
	if s.plat.isIconic(prev) {
		s.plat.restore(prev)
	}
	if err := s.plat.setForeground(prev); err != nil {
		slog.Warn("return focus after overlay", "error", err)
	}
}

func (s *Service) lostFocusOnUI() {
	s.mu.Lock()
	visible := s.visible
	win := s.win
	s.mu.Unlock()
	if !visible {
		return
	}
	if fg := s.plat.foreground(); fg != 0 && fg == win.handle() {
		return
	}
	s.hideOnUI(false)
}
