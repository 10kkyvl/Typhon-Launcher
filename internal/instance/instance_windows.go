//go:build windows

package instance

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hwndMessage = ^uintptr(2) // HWND_MESSAGE is (HWND)-3
	wmCopyData  = 0x004A
	smtoAbort   = 0x0002
	sendTimeout = 5000
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procFindWindowEx       = user32.NewProc("FindWindowExW")
	procSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
)

type copyData struct {
	data   uintptr
	size   uint32
	buffer uintptr
}

type windowsAPI struct{}

func systemAPI() api { return windowsAPI{} }

var claimed []windows.Handle

func (windowsAPI) Claim(name string) (bool, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	h, err := windows.CreateMutex(nil, false, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if err := windows.CloseHandle(h); err != nil {
			return false, fmt.Errorf("close mutex handle: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create mutex: %w", err)
	}
	// The handle is never closed: the mutex must live exactly as long as the process.
	claimed = append(claimed, h)
	return true, nil
}

func (windowsAPI) MutexExists(name string) (bool, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, p)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open mutex: %w", err)
	}
	if err := windows.CloseHandle(h); err != nil {
		return false, fmt.Errorf("close mutex handle: %w", err)
	}
	return true, nil
}

func (windowsAPI) FindWindow(class, title string) (uintptr, error) {
	c, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return 0, err
	}
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}
	//nolint:errcheck,gosec // G103: указатели на строки живут до возврата; Главный принцип: отсутствие окна — штатный исход, FindWindowEx не оставляет по нему отличимого GetLastError.
	r, _, _ := procFindWindowEx.Call(hwndMessage, 0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)))
	return r, nil
}

func (windowsAPI) Send(window uintptr, dataID uintptr, payload string) error {
	text, err := windows.UTF16FromString(payload)
	if err != nil {
		return err
	}
	//nolint:gosec // G103: SendMessageTimeout блокирует до ответа, text и cds живут до конца вызова.
	cds := copyData{data: dataID, size: uint32(len(text) * 2), buffer: uintptr(unsafe.Pointer(&text[0]))}
	//nolint:gosec // G103: SendMessageTimeout блокирует до ответа, text и cds живут до конца вызова.
	r, _, callErr := procSendMessageTimeout.Call(window, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)), smtoAbort, sendTimeout, 0)
	runtime.KeepAlive(text)
	if r == 0 {
		var errno windows.Errno
		if errors.As(callErr, &errno) && errno != 0 && errno != windows.ERROR_TIMEOUT {
			return callErr
		}
		return ErrSendTimeout
	}
	return nil
}
