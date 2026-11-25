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

static bool hf_become_regular(void) {
	return [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
}

// From macOS 14 activation is cooperative: it succeeds when the active app
// yielded to this one (hf_yield in the outgoing launcher) and is refused when
// the user has moved on to another app.
static void hf_activate(void *w) {
	NSWindow *win = (NSWindow *)w;
	if (@available(macOS 14.0, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	[win makeKeyAndOrderFront:nil];
}

// 1 when pid is not a running application yet; before macOS 14 there is
// nothing to yield.
static int hf_yield(int pid) {
	if (@available(macOS 14.0, *)) {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
		if (app == nil) {
			return 1;
		}
		[NSApp yieldActivationToApplication:app];
	}
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The successor is not activated at the reveal: the outgoing launcher is still
// the active app then, and Settle activates once it has exited.
const focusOnReveal = false

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

// becomeRegularApp gives the successor the regular activation policy: a Dock
// tile and a menu bar. It may have run as an accessory app beside the
// outgoing launcher, so the user saw one launcher in the Dock, not two.
func becomeRegularApp() error {
	var ok bool
	application.InvokeSync(func() { ok = bool(C.hf_become_regular()) })
	if !ok {
		return errors.New("the regular activation policy was refused")
	}
	return nil
}

func activateApp(w *application.WebviewWindow) error {
	p, err := nsWindowOf(w)
	if err != nil {
		return err
	}
	application.InvokeSync(func() { C.hf_activate(p) })
	return nil
}

func yieldForeground(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	var rc C.int
	application.InvokeSync(func() { rc = C.hf_yield(C.int(pid)) })
	if rc != 0 {
		return errors.New("the process is not a running application")
	}
	return nil
}

func allowForeground(int) error { return nil }
