package gamemode

import (
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	powrprof              = windows.NewLazySystemDLL("powrprof.dll")
	procPowerGetActive    = powrprof.NewProc("PowerGetActiveScheme")
	procPowerSetActive    = powrprof.NewProc("PowerSetActiveScheme")
	procPowerEnumerate    = powrprof.NewProc("PowerEnumerate")
	accessScheme          = uintptr(16)
	errorNoMoreItems      = uintptr(windows.ERROR_NO_MORE_ITEMS)
	processPriorityAccess = uint32(windows.PROCESS_SET_INFORMATION | windows.PROCESS_QUERY_LIMITED_INFORMATION)
)

type systemPower struct{}

func (systemPower) Active() (string, error) {
	var guid *windows.GUID
	ret, _, callErr := procPowerGetActive.Call(0, uintptr(unsafe.Pointer(&guid))) //nolint:gosec // G103: out pointer to a live local, the call is synchronous
	if ret != 0 {
		slog.Debug("PowerGetActiveScheme failed", "lastError", callErr)
		return "", fmt.Errorf("PowerGetActiveScheme: %w", windows.Errno(ret))
	}
	text := guidString(*guid)
	freeLocal(guid)
	return text, nil
}

func (systemPower) Has(scheme string) (bool, error) {
	want, err := windows.GUIDFromString("{" + scheme + "}")
	if err != nil {
		return false, fmt.Errorf("parse power plan guid: %w", err)
	}
	for i := uintptr(0); ; i++ {
		var found windows.GUID
		size := uint32(unsafe.Sizeof(found))
		ret, _, callErr := procPowerEnumerate.Call(0, 0, 0, accessScheme, i, uintptr(unsafe.Pointer(&found)), uintptr(unsafe.Pointer(&size))) //nolint:gosec // G103: buffers are live locals, the call is synchronous
		if ret == errorNoMoreItems {
			return false, nil
		}
		if ret != 0 {
			slog.Debug("PowerEnumerate failed", "lastError", callErr)
			return false, fmt.Errorf("PowerEnumerate: %w", windows.Errno(ret))
		}
		if found == want {
			return true, nil
		}
	}
}

func (systemPower) Set(scheme string) error {
	guid, err := windows.GUIDFromString("{" + scheme + "}")
	if err != nil {
		return fmt.Errorf("parse power plan guid: %w", err)
	}
	ret, _, callErr := procPowerSetActive.Call(0, uintptr(unsafe.Pointer(&guid))) //nolint:gosec // G103: guid is a live local, the call is synchronous
	if ret != 0 {
		slog.Debug("PowerSetActiveScheme failed", "lastError", callErr)
		return fmt.Errorf("PowerSetActiveScheme: %w", windows.Errno(ret))
	}
	return nil
}

func freeLocal(guid *windows.GUID) {
	if _, err := windows.LocalFree(windows.Handle(unsafe.Pointer(guid))); err != nil { //nolint:gosec // G103: the pointer came from PowerGetActiveScheme and is freed once
		slog.Debug("LocalFree failed", "error", err)
	}
}

func guidString(g windows.GUID) string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

type systemPriority struct{}

func closeHandle(h windows.Handle) {
	if err := windows.CloseHandle(h); err != nil {
		slog.Debug("close process handle failed", "error", err)
	}
}

func (systemPriority) Get(pid uint32) (uint32, error) {
	h, err := windows.OpenProcess(processPriorityAccess, false, pid)
	if err != nil {
		return 0, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer closeHandle(h)
	class, err := windows.GetPriorityClass(h)
	if err != nil {
		return 0, fmt.Errorf("get priority class of %d: %w", pid, err)
	}
	return class, nil
}

func (systemPriority) Set(pid, class uint32) error {
	h, err := windows.OpenProcess(processPriorityAccess, false, pid)
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer closeHandle(h)
	if err := windows.SetPriorityClass(h, class); err != nil {
		return fmt.Errorf("set priority class of %d: %w", pid, err)
	}
	return nil
}

func (systemPriority) Created(pid uint32) (time.Time, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer closeHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return time.Time{}, fmt.Errorf("process times of %d: %w", pid, err)
	}
	return time.Unix(0, created.Nanoseconds()), nil
}

func systemPowerImpl() Power       { return systemPower{} }
func systemPriorityImpl() Priority { return systemPriority{} }
