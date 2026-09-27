//go:build windows

package installguard

import (
	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
	"log/slog"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// Inno implements IAccessible directly but not IDispatch or accDoDefaultAction.
// Read MSAA state, then toggle a checked item through its keyboard handler.
func quietAccessibleOptions(hwnd uintptr) {
	count, ok := message(hwnd, 0x18b, 0, 0) // LB_GETCOUNT
	if !ok || count == 0 || count > 128 {
		return
	}
	var object *ole.IUnknown
	iid := ole.NewGUID("{618736E0-3C3D-11CF-810C-00AA00389B71}")
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
	hr, _, _ := windows.NewLazySystemDLL("oleacc.dll").NewProc("AccessibleObjectFromWindow").Call(hwnd, 0xfffffffc, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&object)))
	if os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" {
		slog.Info("installer checklist", "hwnd", hwnd, "count", count, "accessible", hr)
	}
	//nolint:gosec // G115: HRESULT is a signed 32-bit ABI value returned in a pointer-sized register.
	if int32(hr) < 0 || object == nil {
		return
	}
	defer object.Release()
	for i := uintptr(0); i < count; i++ {
		length, ok := message(hwnd, 0x18a, i, 0) // LB_GETTEXTLEN
		if !ok || length == 0 || length > 2048 {
			continue
		}
		buffer := make([]uint16, length+1)
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		if _, ok = message(hwnd, 0x189, i, uintptr(unsafe.Pointer(&buffer[0]))); !ok {
			continue
		}
		label := windows.UTF16ToString(buffer)
		if !OptionalSiteAction(label) {
			continue
		}
		state, ok := accessibleState(object, int32(i+1))
		if !ok {
			state, ok = wineChecklistState(hwnd, i, count)
		}
		if os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" {
			slog.Info("installer option", "label", label, "state", state, "read", ok)
		}
		if !ok || state&0x10 == 0 || state&1 != 0 {
			continue
		} // CHECKED, not UNAVAILABLE
		old, _ := message(hwnd, 0x188, 0, 0) // LB_GETCURSEL
		message(hwnd, 0x186, i, 0)
		message(hwnd, 0x100, 0x20, 0) // WM_KEYDOWN / VK_SPACE
		message(hwnd, 0x101, 0x20, 0)
		message(hwnd, 0x186, old, 0)
	}
}

func accessibleState(object *ole.IUnknown, child int32) (int64, bool) {
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	table := *(*unsafe.Pointer)(unsafe.Pointer(object))
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	method := *(*uintptr)(unsafe.Add(table, 14*unsafe.Sizeof(uintptr(0)))) // get_accState
	input := ole.NewVariant(ole.VT_I4, int64(child))
	var output ole.VARIANT
	var hr uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
		hr, _, _ = syscall.SyscallN(method, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&input)), uintptr(unsafe.Pointer(&output)))
	} else {
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		words := (*[4]uint32)(unsafe.Pointer(&input))
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
		hr, _, _ = syscall.SyscallN(method, uintptr(unsafe.Pointer(object)), uintptr(words[0]), uintptr(words[1]), uintptr(words[2]), uintptr(words[3]), uintptr(unsafe.Pointer(&output)))
	}
	if os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" {
		slog.Info("installer accState", "hr", hr, "vt", output.VT, "val", output.Val, "child", child)
	}
	runtime.KeepAlive(object)
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer output.Clear()
	//nolint:gosec // G115: HRESULT is a signed 32-bit ABI value returned in a pointer-sized register.
	return output.Val, int32(hr) >= 0 && output.VT == ole.VT_I4
}
