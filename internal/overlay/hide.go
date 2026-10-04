package overlay

import "log/slog"

type hideable interface {
	handle() uintptr
	hide()
	isVisible() bool
}

// A hide that Windows reports as done is not trusted: hiding an owned window
// activates its owner, and a navigating WebView2 can show a window again, so
// the answer comes from IsWindowVisible, not from the call having returned.
func hideVerified(w hideable, what string) bool {
	w.hide()
	if !w.isVisible() {
		return true
	}
	slog.Warn("window is still on screen after hide, hiding it again", "window", what, "hwnd", w.handle(), "visible", true)
	w.hide()
	if !w.isVisible() {
		return true
	}
	slog.Warn("window is still on screen after the second hide, keeping it marked open", "window", what, "hwnd", w.handle(), "visible", true)
	return false
}
