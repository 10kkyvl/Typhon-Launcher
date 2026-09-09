package idle

import (
	"log/slog"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64   = kernel32.NewProc("GetTickCount64")
)

type lastInputInfo struct {
	size uint32
	tick uint32
}

// Since returns the time since the last keyboard or mouse event of the current
// session. The second value is false when the answer is unknown: a caller must
// not read that as "the user is here" or as "the user is away".
func Since() (time.Duration, bool) {
	info := lastInputInfo{}
	info.size = uint32(unsafe.Sizeof(info))
	ret, _, callErr := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info))) //nolint:gosec // G103: указатель на локальную структуру info, живущую весь вызов; конверсия unsafe.Pointer->uintptr внутри самого выражения аргумента Call — единственная форма, разрешённая правилом 4 документации unsafe.Pointer
	if ret == 0 {
		slog.Debug("GetLastInputInfo failed", "error", callErr)
		return 0, false
	}
	now, _, tickErr := procGetTickCount64.Call()
	if now == 0 {
		slog.Debug("GetTickCount64 failed", "error", tickErr)
		return 0, false
	}
	// LASTINPUTINFO.dwTime — 32-битный счётчик тиков, который переполняется
	// каждые ~49.7 суток, поэтому разность считается в 32 битах: иначе
	// переполнение дало бы «простой» длиной в полтора месяца.
	const wrap = uint64(1) << 32
	elapsed := (uint64(now) - uint64(info.tick)) % wrap
	return time.Duration(elapsed) * time.Millisecond, true
}
