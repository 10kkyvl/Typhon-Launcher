package overlay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

const (
	wmQuit   = 0x0012
	wmUser   = 0x0400
	wmHotkey = 0x0312

	modNoRepeat = 0x4000
	hotkeyID    = 1

	pmNoRemove = 0x0000

	monitorDefaultToPrimary = 0x00000001
	monitorDefaultToNearest = 0x00000002

	swpShowWindow = 0x0040
	swRestore     = 9
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procGetMessage          = user32.NewProc("GetMessageW")
	procPeekMessage         = user32.NewProc("PeekMessageW")
	procPostThreadMessage   = user32.NewProc("PostThreadMessageW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
)

// HWND_TOPMOST is (HWND)-1.
var hwndTopmost = ^uintptr(0)

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type winRect struct{ left, top, right, bottom int32 }

type monitorInfo struct {
	size    uint32
	monitor winRect
	work    winRect
	flags   uint32
}

type windowsPlatform struct{}

func newPlatform() platform { return windowsPlatform{} }

func (windowsPlatform) supported() bool { return true }

func lastError(err error, what string) error {
	if err != nil && !errors.Is(err, windows.ERROR_SUCCESS) {
		return fmt.Errorf("%s: %w", what, err)
	}
	return errors.New(what + " failed")
}

//nolint:gosec // G103: PeekMessageW and GetMessageW are handed the address of this goroutine's own MSG, which outlives each call on its locked OS thread.
func (windowsPlatform) register(ctx context.Context, wg *sync.WaitGroup, key hotkey, onPress func()) (func(), error) {
	rctx, cancel := context.WithCancel(ctx)
	type started struct {
		tid uint32
		err error
	}
	ready := make(chan started, 1)
	done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var m msg
		// PostThreadMessageW fails for a thread that has no message queue yet,
		// and the queue is created by the first message call.
		if r1, _, err := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, wmUser, wmUser, pmNoRemove); r1 == 0 && !errors.Is(err, windows.ERROR_SUCCESS) {
			ready <- started{err: lastError(err, "PeekMessageW")}
			return
		}
		if r1, _, err := procRegisterHotKey.Call(0, hotkeyID, uintptr(key.mods|modNoRepeat), uintptr(key.vk)); r1 == 0 {
			ready <- started{err: lastError(err, "RegisterHotKey")}
			return
		}
		defer func() {
			if r1, _, err := procUnregisterHotKey.Call(0, hotkeyID); r1 == 0 {
				slog.Warn("unregister overlay hotkey", "error", lastError(err, "UnregisterHotKey"))
			}
		}()
		ready <- started{tid: windows.GetCurrentThreadId()}

		for {
			r, _, err := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			switch int32(r) {
			case 0:
				return
			case -1:
				slog.Warn("overlay hotkey message loop stopped", "error", lastError(err, "GetMessageW"))
				return
			}
			if m.message == wmHotkey && m.wParam == hotkeyID {
				onPress()
			}
		}
	}()

	res := <-ready
	if res.err != nil {
		cancel()
		<-done
		return nil, res.err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-done:
		case <-rctx.Done():
			if r1, _, err := procPostThreadMessage.Call(uintptr(res.tid), wmQuit, 0, 0); r1 == 0 {
				select {
				case <-done:
				default:
					slog.Warn("stop overlay hotkey loop", "error", lastError(err, "PostThreadMessageW"))
				}
			}
			<-done
		}
	}()

	return func() {
		cancel()
		<-done
	}, nil
}

func (windowsPlatform) foreground() uintptr {
	return uintptr(windows.GetForegroundWindow())
}

//nolint:gosec // G103: GetMonitorInfoW fills the MONITORINFO whose address is passed and which outlives the call. G115: the monitor box is a pixel rectangle that cannot overflow int32.
func (windowsPlatform) monitorRect(hwnd uintptr) (rect, error) {
	flag := uintptr(monitorDefaultToNearest)
	if hwnd == 0 {
		flag = monitorDefaultToPrimary
	}
	mon, _, err := procMonitorFromWindow.Call(hwnd, flag)
	if mon == 0 {
		return rect{}, lastError(err, "MonitorFromWindow")
	}
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r1, _, err := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&info))); r1 == 0 {
		return rect{}, lastError(err, "GetMonitorInfoW")
	}
	m := info.monitor
	return rect{x: m.left, y: m.top, w: m.right - m.left, h: m.bottom - m.top}, nil
}

func (windowsPlatform) setForeground(hwnd uintptr) error {
	if r1, _, err := procSetForegroundWindow.Call(hwnd); r1 == 0 {
		return lastError(err, "SetForegroundWindow")
	}
	return nil
}

func (windowsPlatform) isWindow(hwnd uintptr) bool {
	return windows.IsWindow(windows.HWND(hwnd))
}

// A game in exclusive fullscreen minimises itself when it loses focus, and
// SetForegroundWindow alone does not bring a minimised window back.
//
//nolint:errcheck // IsIconic answers only through its result and sets no last error, so there is no error to check.
func (windowsPlatform) isIconic(hwnd uintptr) bool {
	r1, _, _ := syscall.SyscallN(procIsIconic.Addr(), hwnd)
	return r1 != 0
}

//nolint:errcheck // ShowWindow returns the previous visibility, not success, and sets no last error; a window that stays minimised is caught by the SetForegroundWindow warning that follows.
func (windowsPlatform) restore(hwnd uintptr) {
	syscall.SyscallN(procShowWindow.Addr(), hwnd, swRestore)
}

type wailsWindow struct {
	win *application.WebviewWindow
}

func newWindow(app *application.App) (window, *application.WebviewWindow) {
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: WindowName, Title: "Typhon", URL: WindowURL, Width: 1280, Height: 720,
		Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.RGBA{},
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
	})
	return &wailsWindow{win: win}, win
}

func (w *wailsWindow) handle() uintptr {
	return uintptr(w.win.NativeWindow())
}

// The rectangle is placed with SetWindowPos in physical pixels: Wails' own
// SetPosition and SetSize rescale by the DPI of the monitor the window was on
// before the move, which puts a full-screen window off by the scale factor.
//
//nolint:gosec // G115: SetWindowPos reads the low 32 bits of each argument as a signed int, so a monitor left of the primary one keeps its negative coordinate.
func (w *wailsWindow) show(r rect) error {
	w.win.Run()
	w.win.Show()
	hwnd := w.handle()
	if hwnd == 0 {
		return errors.New("overlay window has no native handle")
	}
	if r1, _, err := procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(r.x), uintptr(r.y), uintptr(r.w), uintptr(r.h), swpShowWindow); r1 == 0 {
		return lastError(err, "SetWindowPos")
	}
	w.win.Focus()
	return nil
}

func (w *wailsWindow) hide() {
	w.win.Hide()
}
