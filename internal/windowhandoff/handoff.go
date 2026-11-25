// Package windowhandoff carries the main window from an outgoing launcher
// process to its successor during a seamless self-update: the old process
// captures the window geometry, the new one applies it while hidden and
// reveals its window in the same place without a system transition.
//
// Platform specifics (disabling show transitions, keeping a hidden webview
// painting, foreground rights) live behind the hooks at the bottom of this
// file; each platform file replaces them. See docs/update-handoff.md.
package windowhandoff

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Geometry is the window state handed over, in logical (DPI-independent) units.
type Geometry struct {
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"w"`
	Height     int     `json:"h"`
	Maximised  bool    `json:"max,omitempty"`
	Fullscreen bool    `json:"fs,omitempty"`
	ScreenID   string  `json:"screen,omitempty"`
	Scale      float32 `json:"scale,omitempty"`
	Focused    bool    `json:"focused,omitempty"`
}

// ErrNoGeometry means a Geometry without a usable size.
var ErrNoGeometry = errors.New("windowhandoff: geometry has no size")

// Valid reports whether g can be applied.
func (g Geometry) Valid() bool { return g.Width > 0 && g.Height > 0 }

// Capture reads the window's current geometry.
func Capture(w *application.WebviewWindow) Geometry {
	x, y := w.Position()
	width, height := w.Size()
	g := Geometry{X: x, Y: y, Width: width, Height: height, Maximised: w.IsMaximised(), Fullscreen: w.IsFullscreen(), Focused: w.IsFocused()}
	if screen, err := w.GetScreen(); err == nil && screen != nil {
		g.ScreenID = screen.ID
		g.Scale = screen.ScaleFactor
	}
	return g
}

// Prepare applies g to a window that is not yet visible and readies it to be
// revealed in place: system show transitions are off and the webview keeps
// painting while the user cannot see it.
func Prepare(w *application.WebviewWindow, g Geometry) error {
	if !g.Valid() {
		return ErrNoGeometry
	}
	if err := disableTransitions(w); err != nil {
		return fmt.Errorf("windowhandoff: disable transitions: %w", err)
	}
	r := place(g, screensFromApp(application.Get()))
	w.SetSize(r.W, r.H)
	w.SetPosition(r.X, r.Y)
	// Maximise after the position so the window maximises on the right screen.
	if g.Maximised && !g.Fullscreen {
		w.Maximise()
	}
	if err := paintHidden(w); err != nil {
		return fmt.Errorf("windowhandoff: paint hidden: %w", err)
	}
	if g.Fullscreen {
		// On macOS native fullscreen always plays the Space-change animation;
		// there is no supported way around it, so the user sees one system
		// transition and we do not try to bypass it.
		w.Fullscreen()
	}
	return nil
}

// Preparer readies a successor's window as soon as its native window exists.
// The native window is created inside the application's run loop, after the
// window object: an attempt made before that is not a failure, a later one
// does the work. Unprepared, the window is never ordered in and its webview
// draws nothing, so the outgoing launcher would wait for a frame that cannot
// come.
type Preparer struct {
	done    atomic.Bool
	ready   func() bool
	prepare func() error
}

func NewPreparer(w *application.WebviewWindow, g Geometry) *Preparer {
	return &Preparer{
		ready:   func() bool { return w.NativeWindow() != nil },
		prepare: func() error { return Prepare(w, g) },
	}
}

// Try prepares the window if its native window is there and no earlier call
// has prepared it. prepared tells whether this call did the work; err is that
// work's result.
func (p *Preparer) Try() (prepared bool, err error) {
	if !p.ready() {
		return false, nil
	}
	if !p.done.CompareAndSwap(false, true) {
		return false, nil
	}
	return true, p.prepare()
}

// DisableTransitions turns off the system show/hide/close animation of w, so
// the outgoing window can be closed without a visible effect. Reveal re-enables
// animations on the successor.
func DisableTransitions(w *application.WebviewWindow) error {
	if err := disableTransitions(w); err != nil {
		return fmt.Errorf("windowhandoff: disable transitions: %w", err)
	}
	return nil
}

// Reveal makes a prepared window visible without a system transition. focus
// brings it to the front; pass it only when the outgoing window was active.
// On macOS the application is not activated here: that waits for Settle, once
// the outgoing launcher is gone.
func Reveal(w *application.WebviewWindow, focus bool) error {
	if err := reveal(w); err != nil {
		return fmt.Errorf("windowhandoff: reveal: %w", err)
	}
	if focus && focusOnReveal {
		w.Focus()
	}
	return nil
}

// The platform calls of Settle and YieldForeground; tests replace them.
var (
	becomeRegular = becomeRegularApp
	activate      = activateApp
	yieldTo       = yieldForeground
)

// Settle makes the successor an ordinary application once the outgoing
// launcher has exited: a regular app with its own Dock tile and menu bar and,
// when focus is set (the outgoing window was the active one), the active app
// with w as its key window. Without focus nothing is activated, so whatever
// the user moved to keeps the focus.
func Settle(w *application.WebviewWindow, focus bool) error {
	if err := becomeRegular(); err != nil {
		return fmt.Errorf("windowhandoff: become a regular app: %w", err)
	}
	if !focus {
		return nil
	}
	if err := activate(w); err != nil {
		return fmt.Errorf("windowhandoff: activate: %w", err)
	}
	return nil
}

// YieldForeground lets the successor process pid take the activation from
// this one after it has exited (macOS cooperative activation); a no-op
// elsewhere and on macOS before 14.
func YieldForeground(pid int) error {
	if err := yieldTo(pid); err != nil {
		return fmt.Errorf("windowhandoff: yield the foreground to %d: %w", pid, err)
	}
	return nil
}

// AllowForeground lets process pid take the foreground (Windows); a no-op elsewhere.
func AllowForeground(pid int) error { return allowForeground(pid) }

// Encode packs v for a single command-line argument.
func Encode(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("windowhandoff: encode: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Decode unpacks an argument made by Encode into v.
func Decode(arg string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(arg)
	if err != nil {
		return fmt.Errorf("windowhandoff: decode: %w", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("windowhandoff: decode: %w", err)
	}
	return nil
}
