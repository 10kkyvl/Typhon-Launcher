//go:build darwin

package windowhandoff

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// All functions must run on the main thread.

static void hf_set_transitions(void *w, int disabled) {
	NSWindow *win = (NSWindow *)w;
	win.animationBehavior = disabled ? NSWindowAnimationBehaviorNone : NSWindowAnimationBehaviorDefault;
}

// Alpha 0 with mouse events ignored, ordered in without becoming key and
// without activating the application.
static void hf_paint_hidden(void *w) {
	NSWindow *win = (NSWindow *)w;
	win.alphaValue = 0.0;
	win.ignoresMouseEvents = YES;
	[win orderFrontRegardless];
}

static void hf_reveal(void *w) {
	NSWindow *win = (NSWindow *)w;
	win.ignoresMouseEvents = NO;
	[win orderFrontRegardless];
	win.alphaValue = 1.0;
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func nsWindowOf(w *application.WebviewWindow) (unsafe.Pointer, error) {
	p := w.NativeWindow()
	if p == nil {
		return nil, errors.New("window has no native handle yet")
	}
	return p, nil
}

func disableTransitions(w *application.WebviewWindow) error {
	p, err := nsWindowOf(w)
	if err != nil {
		return err
	}
	application.InvokeSync(func() { C.hf_set_transitions(p, 1) })
	return nil
}

// paintHidden orders the window in at alpha 0 so WKWebView keeps rendering
// (a window that is not ordered in has no visible surface and WebKit stops
// requestAnimationFrame).
func paintHidden(w *application.WebviewWindow) error {
	p, err := nsWindowOf(w)
	if err != nil {
		return err
	}
	application.InvokeSync(func() { C.hf_paint_hidden(p) })
	return nil
}

func reveal(w *application.WebviewWindow) error {
	p, err := nsWindowOf(w)
	if err != nil {
		return err
	}
	application.InvokeSync(func() {
		C.hf_reveal(p)
		C.hf_set_transitions(p, 0)
	})
	return nil
}

func allowForeground(int) error { return nil }
