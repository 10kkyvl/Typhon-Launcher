//go:build !windows && !darwin

package windowhandoff

import "github.com/wailsapp/wails/v3/pkg/application"

// Platforms without native hooks: a plain Show is the best available reveal.

const focusOnReveal = true

func disableTransitions(*application.WebviewWindow) error { return nil }

func paintHidden(*application.WebviewWindow) error { return nil }

func reveal(w *application.WebviewWindow) error {
	w.Show()
	return nil
}

func becomeRegularApp() error { return nil }

func activateApp(*application.WebviewWindow) error { return nil }

func yieldForeground(int) error { return nil }

func allowForeground(int) error { return nil }
