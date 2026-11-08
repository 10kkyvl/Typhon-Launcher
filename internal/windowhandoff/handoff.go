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
func Reveal(w *application.WebviewWindow, focus bool) error {
	if err := reveal(w); err != nil {
		return fmt.Errorf("windowhandoff: reveal: %w", err)
	}
	if focus {
		w.Focus()
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
