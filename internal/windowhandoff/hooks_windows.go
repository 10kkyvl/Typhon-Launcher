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
	//nolint:gosec // G103: address of the local v, live for the call. G115: an HRESULT is a 32-bit pattern, printed as one.
	if hr := w32.DwmSetWindowAttribute(hwnd, dwmwaTransitionsForceDisabled, unsafe.Pointer(&v), unsafe.Sizeof(v)); hr != 0 {
		return fmt.Errorf("DwmSetWindowAttribute(TRANSITIONS_FORCEDISABLED): HRESULT 0x%x", uint32(hr)) //nolint:gosec // G115: as above
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
		old := uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)) //nolint:gosec // G115: the extended style is a 32-bit mask read through a signed API
		savedMu.Lock()
		if _, ok := savedStyle[hwnd]; !ok {
			savedStyle[hwnd] = old
		}
		savedMu.Unlock()
		w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, old|hiddenStyle)
		if !w32.SetLayeredWindowAttributes(hwnd, 0, 0, w32.LWA_ALPHA) {
			// Leave an ordinary window for the plain Show the caller falls
			// back to: a layered one without attributes stays invisible.
			w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, old)
			savedMu.Lock()
			delete(savedStyle, hwnd)
			savedMu.Unlock()
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
		return revealStyled(win32Window(hwnd), orig, ok, hiddenStyle)
	})
}

// win32Window is styleOps on a real window handle.
type win32Window w32.HWND

func (h win32Window) exStyle() uint32 {
	return uint32(w32.GetWindowLong(w32.HWND(h), w32.GWL_EXSTYLE)) //nolint:gosec // G115: the extended style is a 32-bit mask read through a signed API
}

func (h win32Window) setExStyle(style uint32) {
	w32.SetWindowLong(w32.HWND(h), w32.GWL_EXSTYLE, style)
}

func (h win32Window) setAlpha(alpha byte) error {
	if !w32.SetLayeredWindowAttributes(w32.HWND(h), 0, alpha, w32.LWA_ALPHA) {
		return fmt.Errorf("SetLayeredWindowAttributes(alpha %d) failed", alpha)
	}
	return nil
}

func (h win32Window) setTransitions(disabled bool) error {
	return setTransitions(w32.HWND(h), disabled)
}

func (h win32Window) frameChanged() error {
	if !w32.SetWindowPos(w32.HWND(h), 0, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE|w32.SWP_FRAMECHANGED) {
		return errors.New("SetWindowPos(FRAMECHANGED) failed")
	}
	return nil
}

// The successor takes the focus at the reveal, while the outgoing launcher
// still holds the foreground right it granted (allowForeground); Windows has
// no activation policy to settle afterwards.
const focusOnReveal = true

func becomeRegularApp() error { return nil }

func activateApp(*application.WebviewWindow) error { return nil }

func yieldForeground(int) error { return nil }

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
