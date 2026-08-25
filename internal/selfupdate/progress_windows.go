//go:build windows

package selfupdate

import (
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy        = 0x0002
	wmPaint          = 0x000F
	wmClose          = 0x0010
	wmEraseBkgnd     = 0x0014
	wmTimer          = 0x0113
	wmApp            = 0x8000
	csHRedraw        = 0x0002
	csVRedraw        = 0x0001
	wsCaption        = 0x00C00000
	wsSysMenu        = 0x00080000
	wsVisible        = 0x10000000
	wsExTopmost      = 0x00000008
	wsExAppWindow    = 0x00040000
	swShow           = 5
	idcArrow         = 32512
	dtSingleLine     = 0x0020
	dtWordBreak      = 0x0010
	dtNoPrefix       = 0x0800
	dtEndEllipsis    = 0x8000
	bkTransparent    = 1
	srcCopy          = 0x00CC0020
	smCXScreen       = 0
	smCYScreen       = 1
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	logPixelsX       = 88
	animTimerID      = 1
	animInterval     = 33
	fontWeightMed    = 600
	fontWeightReg    = 400
	defaultCharSet   = 1
	cleartypeQuality = 5
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPostMessage      = user32.NewProc("PostMessageW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procInvalidateRect   = user32.NewProc("InvalidateRect")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procFillRect         = user32.NewProc("FillRect")
	procDrawText         = user32.NewProc("DrawTextW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procSetTimer         = user32.NewProc("SetTimer")
	procKillTimer        = user32.NewProc("KillTimer")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procGetDpiForSystem  = user32.NewProc("GetDpiForSystem")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")

	procSetWindowAttr = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procCreateFont       = gdi32.NewProc("CreateFontW")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt           = gdi32.NewProc("BitBlt")
	procDeleteDC         = gdi32.NewProc("DeleteDC")
	procGetDeviceCaps    = gdi32.NewProc("GetDeviceCaps")
)

// call is the single place this file talks to user32/gdi32. Every entry point
// here draws or moves the progress window and has no recovery path worth
// taking: the update must run whether or not it draws, and the two callers
// whose failure does matter (RegisterClassExW, CreateWindowExW) check the
// returned handle instead and abandon the window.
//
//nolint:errcheck // LazyProc.Call returns a non-nil error built from GetLastError on every call, stale unless the call itself failed; the drawing calls routed through here have no failure mode the worker can act on.
func call(proc *windows.LazyProc, args ...uintptr) uintptr {
	ret, _, _ := proc.Call(args...)
	return ret
}

type rect struct {
	left, top, right, bottom int32
}

type point struct {
	x, y int32
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

var (
	colorBackground = rgb(0x0b, 0x0f, 0x14)
	colorTitle      = rgb(0xee, 0xf2, 0xf6)
	colorDetail     = rgb(0x9b, 0xa6, 0xb2)
	colorTrack      = rgb(0x19, 0x21, 0x2b)
	colorAccent     = rgb(0x68, 0x75, 0xe8)
	colorDanger     = rgb(0xd9, 0x69, 0x69)
)

// progressUI is the only window the worker owns, and the only thing the user
// sees between the launcher quitting and the new build starting. Every method
// tolerates a window that never appeared: a missing window must not stop the
// update itself.
type progressUI struct {
	mu       sync.Mutex
	hwnd     uintptr
	title    string
	detail   string
	failed   bool
	phase    int32
	ready    chan struct{}
	finished chan struct{}
}

func newProgressUI(title, detail string) *progressUI {
	ui := &progressUI{
		title:    title,
		detail:   detail,
		ready:    make(chan struct{}),
		finished: make(chan struct{}),
	}
	go ui.run()
	<-ui.ready
	return ui
}

func (u *progressUI) setStage(title, detail string) {
	u.mu.Lock()
	u.title, u.detail = title, detail
	hwnd := u.hwnd
	u.mu.Unlock()
	post(hwnd, wmApp)
}

func (u *progressUI) fail(title, detail string) {
	u.mu.Lock()
	u.title, u.detail, u.failed = title, detail, true
	hwnd := u.hwnd
	u.mu.Unlock()
	post(hwnd, wmApp)
}

// wait blocks until the user closes the window. The worker uses it only when
// there is no launcher left to report through: an update that failed before
// the relaunch would otherwise vanish without a trace.
func (u *progressUI) wait() {
	<-u.finished
}

func (u *progressUI) close() {
	u.mu.Lock()
	hwnd := u.hwnd
	u.mu.Unlock()
	post(hwnd, wmClose)
	<-u.finished
}

func post(hwnd uintptr, message uintptr) {
	if hwnd == 0 {
		return
	}
	if _, _, err := procPostMessage.Call(hwnd, message, 0, 0); !errors.Is(err, windows.ERROR_SUCCESS) {
		slog.Warn("post to selfupdate progress window", "message", message, "error", err)
	}
}

var activeUI struct {
	sync.Mutex
	ui *progressUI
}

func (u *progressUI) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(u.finished)

	if err := u.createWindow(); err != nil {
		slog.Warn("selfupdate progress window unavailable", "error", err)
		close(u.ready)
		return
	}
	close(u.ready)
	u.pump()
}

//nolint:gosec // G103: a message loop has to hand GetMessageW the address of its own MSG; the value lives on this goroutine's locked OS thread for the whole loop.
func (u *progressUI) pump() {
	var m winMsg
	for {
		if int32(call(procGetMessage, uintptr(unsafe.Pointer(&m)), 0, 0, 0)) <= 0 {
			return
		}
		call(procTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(procDispatchMessage, uintptr(unsafe.Pointer(&m)))
	}
}

//nolint:gosec // G103: RegisterClassExW and CreateWindowExW take pointers to the class record and the UTF-16 names, all of which outlive the call. G115: the screen metrics and the window box are pixel counts that cannot overflow int32 on any real display.
func (u *progressUI) createWindow() error {
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return err
	}
	className, err := windows.UTF16PtrFromString("TyphonSelfUpdateProgress")
	if err != nil {
		return err
	}
	windowTitle, err := windows.UTF16PtrFromString("Обновление Typhon")
	if err != nil {
		return err
	}

	cursor := call(procLoadCursor, 0, idcArrow)
	icon := call(procLoadIcon, uintptr(instance), 1)

	activeUI.Lock()
	activeUI.ui = u
	activeUI.Unlock()

	class := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         csHRedraw | csVRedraw,
		lpfnWndProc:   windows.NewCallback(windowProc),
		hInstance:     uintptr(instance),
		hIcon:         icon,
		hCursor:       cursor,
		lpszClassName: className,
		hIconSm:       icon,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return callErr
	}

	scale := systemScale()
	width := int32(460 * scale)
	height := int32(200 * scale)
	x := (int32(call(procGetSystemMetrics, smCXScreen)) - width) / 2
	y := (int32(call(procGetSystemMetrics, smCYScreen)) - height) / 2

	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		wsCaption|wsSysMenu|wsVisible,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, uintptr(instance), 0,
	)
	if hwnd == 0 {
		return callErr
	}

	u.mu.Lock()
	u.hwnd = hwnd
	u.mu.Unlock()

	darkTitleBar(hwnd)
	call(procSetWindowPos, hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoZOrder|swpNoActivate)
	call(procShowWindow, hwnd, swShow)
	call(procUpdateWindow, hwnd)
	call(procSetForeground, hwnd)
	call(procSetTimer, hwnd, animTimerID, animInterval, 0)
	return nil
}

// darkTitleBar keeps the frame from clashing with the dark body. The attribute
// only exists from Windows 10 20H1 on and the call is advisory, so an older
// build simply keeps the light frame.
//
//nolint:errcheck,gosec // G103/errcheck: DwmSetWindowAttribute takes the address of the flag it reads during the call, and a frame that stays light is not a failure the worker acts on.
func darkTitleBar(hwnd uintptr) {
	const useImmersiveDarkMode = 20
	enabled := int32(1)
	procSetWindowAttr.Call(hwnd, useImmersiveDarkMode, uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled))
}

func systemScale() float64 {
	if err := procGetDpiForSystem.Find(); err == nil {
		if dpi := call(procGetDpiForSystem); dpi > 0 {
			return float64(dpi) / 96
		}
	}
	dc := call(procGetDC, 0)
	if dc == 0 {
		return 1
	}
	defer call(procReleaseDC, 0, dc)
	dpi := call(procGetDeviceCaps, dc, logPixelsX)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	activeUI.Lock()
	ui := activeUI.ui
	activeUI.Unlock()

	switch message {
	case wmEraseBkgnd:
		return 1
	case wmApp:
		call(procInvalidateRect, hwnd, 0, 0)
		return 0
	case wmTimer:
		if ui != nil {
			ui.mu.Lock()
			if !ui.failed {
				ui.phase++
			}
			ui.mu.Unlock()
			call(procInvalidateRect, hwnd, 0, 0)
		}
		return 0
	case wmPaint:
		if ui != nil {
			ui.paint(hwnd)
			return 0
		}
	case wmClose:
		call(procKillTimer, hwnd, animTimerID)
		call(procDestroyWindow, hwnd)
		return 0
	case wmDestroy:
		call(procPostQuitMessage, 0)
		return 0
	}
	return call(procDefWindowProc, hwnd, uintptr(message), wParam, lParam)
}

//nolint:gosec // G103: BeginPaint/GetClientRect fill structs this frame owns and does not outlive. G115: every value converted here is a pixel count taken from the window this process just sized.
func (u *progressUI) paint(hwnd uintptr) {
	var ps paintStruct
	hdc := call(procBeginPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer call(procEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))

	var client rect
	call(procGetClientRect, hwnd, uintptr(unsafe.Pointer(&client)))
	width, height := client.right, client.bottom
	if width <= 0 || height <= 0 {
		return
	}

	mem := call(procCreateCompatDC, hdc)
	if mem == 0 {
		return
	}
	defer call(procDeleteDC, mem)
	bitmap := call(procCreateCompatBmp, hdc, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return
	}
	defer call(procDeleteObject, bitmap)
	defer call(procSelectObject, mem, call(procSelectObject, mem, bitmap))

	u.mu.Lock()
	title, detail, failed, phase := u.title, u.detail, u.failed, u.phase
	u.mu.Unlock()

	scale := systemScale()
	px := func(v float64) int32 { return int32(v * scale) }

	fillRect(mem, rect{0, 0, width, height}, colorBackground)

	pad := px(28)
	titleFont := createFont(px(19), fontWeightMed)
	defer call(procDeleteObject, titleFont)
	detailFont := createFont(px(14), fontWeightReg)
	defer call(procDeleteObject, detailFont)

	call(procSetBkMode, mem, bkTransparent)

	titleColor := colorTitle
	if failed {
		titleColor = colorDanger
	}
	drawText(mem, titleFont, titleColor, title, rect{pad, px(26), width - pad, px(58)}, dtSingleLine|dtNoPrefix|dtEndEllipsis)
	drawText(mem, detailFont, colorDetail, detail, rect{pad, px(66), width - pad, height - px(24)}, dtWordBreak|dtNoPrefix)

	if !failed {
		fillBar(mem, rect{pad, height - px(46), width - pad, height - px(40)}, phase*px(7))
	}

	call(procBitBlt, hdc, 0, 0, uintptr(width), uintptr(height), mem, 0, 0, srcCopy)
}

// fillBar draws the indeterminate stripe: the installer reports no progress of
// its own, so the window shows that something is still happening rather than a
// percentage it would have to invent.
func fillBar(dc uintptr, bar rect, offset int32) {
	fillRect(dc, bar, colorTrack)
	barWidth := bar.right - bar.left
	segment := barWidth / 3
	if segment <= 0 {
		return
	}
	left := bar.left + offset%(barWidth+segment) - segment
	right := left + segment
	if left < bar.left {
		left = bar.left
	}
	if right > bar.right {
		right = bar.right
	}
	if right > left {
		fillRect(dc, rect{left, bar.top, right, bar.bottom}, colorAccent)
	}
}

//nolint:gosec // G103: FillRect takes the address of a rectangle that lives for the duration of the call.
func fillRect(dc uintptr, r rect, color uintptr) {
	brush := call(procCreateSolidBrush, color)
	if brush == 0 {
		return
	}
	defer call(procDeleteObject, brush)
	call(procFillRect, dc, uintptr(unsafe.Pointer(&r)), brush)
}

//nolint:gosec // G103: DrawTextW takes the address of the UTF-16 buffer and of the layout rectangle, both of which live for the duration of the call.
func drawText(dc, font, color uintptr, text string, r rect, format uintptr) {
	if text == "" {
		return
	}
	encoded, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	defer call(procSelectObject, dc, call(procSelectObject, dc, font))
	call(procSetTextColor, dc, color)
	call(procDrawText, dc, uintptr(unsafe.Pointer(&encoded[0])), uintptr(len(encoded)-1), uintptr(unsafe.Pointer(&r)), format)
}

//nolint:gosec // G103: CreateFontW takes the address of the face name, which lives for the duration of the call. G115: height and weight are small constants scaled by the display DPI.
func createFont(height, weight int32) uintptr {
	face, err := windows.UTF16PtrFromString("Segoe UI")
	if err != nil {
		return 0
	}
	return call(procCreateFont,
		uintptr(-height), 0, 0, 0, uintptr(weight),
		0, 0, 0,
		defaultCharSet, 0, 0, cleartypeQuality, 0,
		uintptr(unsafe.Pointer(face)),
	)
}
