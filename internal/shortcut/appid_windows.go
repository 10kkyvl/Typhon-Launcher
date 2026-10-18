//go:build windows

package shortcut

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSetCurrentProcessExplicitAppUserModelID = windows.NewLazySystemDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID")

// SetProcessAppID задаёт процессу явный AppUserModelID. Без него оболочка
// выводит идентичность окна из ярлыков на тот же exe и берёт значок игры.
//
//nolint:gosec // G103: address of the UTF-16 buffer, live for the call. G115: HRESULT is the 32-bit value in r0.
func SetProcessAppID(id string) error {
	if err := ValidateAppID(id); err != nil {
		return err
	}
	if err := procSetCurrentProcessExplicitAppUserModelID.Find(); err != nil {
		return fmt.Errorf("shortcut: shell32!SetCurrentProcessExplicitAppUserModelID: %w", err)
	}
	ptr, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return fmt.Errorf("shortcut: SetCurrentProcessExplicitAppUserModelID: кодирование %q: %w", id, err)
	}
	r0 := callMethod(procSetCurrentProcessExplicitAppUserModelID.Addr(), uintptr(unsafe.Pointer(ptr)))
	if hr := int32(r0); hr < 0 {
		return hresultErr("SetCurrentProcessExplicitAppUserModelID", hr)
	}
	return nil
}
