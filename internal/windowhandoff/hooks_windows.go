//go:build windows

package windowhandoff

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

const dwmwaTransitionsForceDisabled = 3 // DWMWA_TRANSITIONS_FORCEDISABLED

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

// hiddenStyle is the extended style added while the window paints invisibly:
// layered (alpha 0), click-through and never activated.
const hiddenStyle = w32.WS_EX_LAYERED | w32.WS_EX_TRANSPARENT | w32.WS_EX_NOACTIVATE

// savedStyle remembers the extended-style bits the window had before
// paintHidden, so reveal puts back exactly what Wails (or the app) set.
var (
	savedMu    sync.Mutex
	savedStyle = map[w32.HWND]uint32{}
)

func hwndOf(w *application.WebviewWindow) (w32.HWND, error) {
	p := w.NativeWindow()
	if p == nil {
		return 0, errors.New("window has no native handle yet")
	}
	return w32.HWND(uintptr(p)), nil
}

func setTransitions(hwnd w32.HWND, disabled bool) error {
	var v int32
	if disabled {
		v = 1
	}
	if hr := w32.DwmSetWindowAttribute(hwnd, dwmwaTransitionsForceDisabled, unsafe.Pointer(&v), unsafe.Sizeof(v)); hr != 0 {
		return fmt.Errorf("DwmSetWindowAttribute(TRANSITIONS_FORCEDISABLED): HRESULT 0x%x", uint32(hr))
	}
	return nil
}

func disableTransitions(w *application.WebviewWindow) error {
	hwnd, err := hwndOf(w)
	if err != nil {
		return err
	}
	return application.InvokeSyncWithError(func() error { return setTransitions(hwnd, true) })
}

// paintHidden shows the window with alpha 0. The window must count as shown
// for WebView2 (controller visible) or it throttles rendering and requestAnimationFrame
// never fires; alpha 0 keeps it invisible while DWM still composes it.
// Wails' own show() does the bookkeeping (windowShown, chromium.Show) but uses
// SW_SHOW; WS_EX_NOACTIVATE keeps that from stealing focus and
// WS_EX_TRANSPARENT keeps the invisible window from swallowing clicks.
func paintHidden(w *application.WebviewWindow) error {
	hwnd, err := hwndOf(w)
	if err != nil {
		return err
	}
	return application.InvokeSyncWithError(func() error {
		old := uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE))
		savedMu.Lock()
		if _, ok := savedStyle[hwnd]; !ok {
			savedStyle[hwnd] = old
		}
		savedMu.Unlock()
		w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, old|hiddenStyle)
		if !w32.SetLayeredWindowAttributes(hwnd, 0, 0, w32.LWA_ALPHA) {
			return errors.New("SetLayeredWindowAttributes(alpha 0) failed")
		}
		w.Show()
		return nil
	})
}

// reveal makes the window opaque and active again, restores its original
// extended style and re-enables the system animations (so later minimise and
// maximise behave normally). The window is already shown, so DWM plays no show
// transition.
func reveal(w *application.WebviewWindow) error {
	hwnd, err := hwndOf(w)
	if err != nil {
		return err
	}
	return application.InvokeSyncWithError(func() error {
		savedMu.Lock()
		orig, ok := savedStyle[hwnd]
		delete(savedStyle, hwnd)
		savedMu.Unlock()
		if !ok {
			orig = uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)) &^ hiddenStyle
		}
		var errs []error
		if !w32.SetLayeredWindowAttributes(hwnd, 0, 255, w32.LWA_ALPHA) {
			errs = append(errs, errors.New("SetLayeredWindowAttributes(alpha 255) failed"))
		}
		// Dropping WS_EX_LAYERED (unless the app had it) makes the window an
		// ordinary one again; the others are restored from the saved style.
		w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, orig)
		if err := setTransitions(hwnd, false); err != nil {
			errs = append(errs, err)
		}
		// Repaint the frame after the style change.
		if !w32.SetWindowPos(hwnd, 0, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_FRAMECHANGED) {
			errs = append(errs, errors.New("SetWindowPos(FRAMECHANGED) failed"))
		}
		return errors.Join(errs...)
	})
}

// allowForeground lets process pid call SetForegroundWindow, so the successor
// can take focus from the outgoing launcher when the user was in it.
func allowForeground(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	if err := procAllowSetForegroundWindow.Find(); err != nil {
		return fmt.Errorf("user32.AllowSetForegroundWindow: %w", err)
	}
	r1, _, callErr := procAllowSetForegroundWindow.Call(uintptr(pid))
	if r1 == 0 {
		return fmt.Errorf("AllowSetForegroundWindow(%d): %w", pid, callErr)
	}
	return nil
}
