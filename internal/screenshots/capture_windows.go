package screenshots

import (
	"errors"
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dwmwaExtendedFrameBounds = 9

	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000

	hgdiError = ^uintptr(0)

	maxSide  = 16384
	maxBytes = 256 << 20

	biRGB        = 0
	dibRGBColors = 0
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	procWPID = user32.NewProc("GetWindowThreadProcessId")
	procRect = user32.NewProc("GetWindowRect")
	procIsW  = user32.NewProc("IsWindow")
	procIcon = user32.NewProc("IsIconic")
	procGDC  = user32.NewProc("GetDC")
	procRDC  = user32.NewProc("ReleaseDC")
	procDwm  = dwmapi.NewProc("DwmGetWindowAttribute")
	procCDC  = gdi32.NewProc("CreateCompatibleDC")
	procCBM  = gdi32.NewProc("CreateCompatibleBitmap")
	procSel  = gdi32.NewProc("SelectObject")
	procBlt  = gdi32.NewProc("BitBlt")
	procBits = gdi32.NewProc("GetDIBits")
	procDelO = gdi32.NewProc("DeleteObject")
	procDelD = gdi32.NewProc("DeleteDC")
)

type winRect struct{ left, top, right, bottom int32 }

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

func gdiError(op string, err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno != 0 {
		return fmt.Errorf("%w: %s: code %d: %w", ErrGDI, op, uint32(errno), err)
	}
	return fmt.Errorf("%w: %s", ErrGDI, op)
}

func windowPID(hwnd uintptr) (uint32, error) {
	var pid uint32
	//nolint:gosec // G103: GetWindowThreadProcessId writes the process id through the address of a local that outlives the call.
	tid, _, err := procWPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if tid == 0 {
		return 0, gdiError("GetWindowThreadProcessId", err)
	}
	return pid, nil
}

//nolint:gosec // G103: both calls fill the RECT whose address is passed and which outlives the call.
func windowArea(hwnd uintptr) (winRect, error) {
	var r winRect
	hr, _, _ := procDwm.Call(hwnd, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r)) //nolint:errcheck // errcheck: DwmGetWindowAttribute reports through its HRESULT, which is checked, not through last error.
	if int32(hr) >= 0 {
		return r, nil
	}
	if r1, _, err := procRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); r1 == 0 {
		return winRect{}, gdiError("GetWindowRect", err)
	}
	return r, nil
}

func grabWindow(hwnd uintptr) (grabbed, error) {
	if hwnd == 0 {
		return grabbed{}, ErrNoWindow
	}
	if live, _, _ := procIsW.Call(hwnd); live == 0 { //nolint:errcheck // errcheck: IsWindow answers through its BOOL result and sets no last error.
		return grabbed{}, ErrNoWindow
	}
	if iconic, _, _ := procIcon.Call(hwnd); iconic != 0 { //nolint:errcheck // errcheck: IsIconic answers through its BOOL result and sets no last error.
		return grabbed{}, fmt.Errorf("%w: window is minimized", ErrNoWindow)
	}
	pid, err := windowPID(hwnd)
	if err != nil {
		return grabbed{}, err
	}
	area, err := windowArea(hwnd)
	if err != nil {
		return grabbed{}, err
	}
	w, h := area.right-area.left, area.bottom-area.top
	if w <= 0 || h <= 0 {
		return grabbed{}, fmt.Errorf("%w: empty window area %dx%d", ErrNoWindow, w, h)
	}
	if w > maxSide || h > maxSide || int64(w)*int64(h)*4 > maxBytes {
		return grabbed{}, fmt.Errorf("%w: %dx%d", ErrTooLarge, w, h)
	}
	img, err := blit(area.left, area.top, int(w), int(h))
	if err != nil {
		return grabbed{}, err
	}
	return grabbed{img: img, pid: pid}, nil
}

//nolint:gosec // G103: GetDIBits fills the pixel buffer and BITMAPINFOHEADER whose addresses are passed and which outlive the call. G115: the window area is a pixel box that cannot overflow the integer widths the GDI calls take.
func blit(x, y int32, w, h int) (*image.RGBA, error) {
	screen, _, err := procGDC.Call(0)
	if screen == 0 {
		return nil, gdiError("GetDC", err)
	}
	defer procRDC.Call(0, screen) //nolint:errcheck // errcheck: ReleaseDC runs on the way out and its result cannot change an already decided capture.

	mem, _, err := procCDC.Call(screen)
	if mem == 0 {
		return nil, gdiError("CreateCompatibleDC", err)
	}
	defer procDelD.Call(mem) //nolint:errcheck // errcheck: DeleteDC runs on the way out and its result cannot change an already decided capture.

	bmp, _, err := procCBM.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, gdiError("CreateCompatibleBitmap", err)
	}
	defer procDelO.Call(bmp) //nolint:errcheck // errcheck: DeleteObject runs on the way out and its result cannot change an already decided capture.

	old, _, err := procSel.Call(mem, bmp)
	if old == 0 || old == hgdiError {
		return nil, gdiError("SelectObject", err)
	}

	blt, _, bltErr := procBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(x), uintptr(y), srcCopy|captureBlt)
	// GetDIBits needs the bitmap out of every DC, and DeleteObject cannot free one that is still selected.
	back, _, backErr := procSel.Call(mem, old)
	if blt == 0 {
		return nil, gdiError("BitBlt", bltErr)
	}
	if back == 0 || back == hgdiError {
		return nil, gdiError("SelectObject restore", backErr)
	}

	header := bitmapInfoHeader{
		size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:       int32(w),
		height:      -int32(h),
		planes:      1,
		bitCount:    32,
		compression: biRGB,
	}
	pix := make([]byte, w*h*4)
	if r1, _, err := procBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&header)), dibRGBColors); r1 == 0 {
		return nil, gdiError("GetDIBits", err)
	}

	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
		pix[i+3] = 0xFF
	}
	return &image.RGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}
