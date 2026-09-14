package messaging

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log/slog"
	"syscall"
	"unsafe"
)

var chatUser32 = syscall.NewLazyDLL("user32.dll")
var chatGetStyle = chatUser32.NewProc("GetWindowLongPtrW")
var chatSetStyle = chatUser32.NewProc("SetWindowLongPtrW")
var chatPlaySound = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")

func showWithoutActivation(w *application.WebviewWindow) {
	application.InvokeSync(func() {
		hwnd := uintptr(w.NativeWindow())
		if hwnd != 0 {
			// WS_EX_NOACTIVATE makes Wails' SW_SHOW leave the foreground game alone.
			index := ^uintptr(19) // GWL_EXSTYLE (-20) in the Win32 integer argument.
			style, _, getErr := chatGetStyle.Call(hwnd, index)
			if style == 0 && !errors.Is(getErr, syscall.Errno(0)) {
				slog.Debug("read chat window style", "error", getErr)
				return
			}
			previous, _, setErr := chatSetStyle.Call(hwnd, index, style|0x08000000)
			if previous == 0 && !errors.Is(setErr, syscall.Errno(0)) {
				slog.Debug("set chat window style", "error", setErr)
			}
		}
	})
	w.Show()
}
func playTone(ctx context.Context, path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil || ctx.Err() != nil {
		return
	}
	// SND_FILENAME | SND_NODEFAULT. Synchronous playback stays in the tracked
	// worker; the sound lasts only 420ms and needs no external executable.
	// #nosec G103 -- PlaySoundW reads this valid NUL-terminated UTF-16 buffer synchronously before the call returns.
	played, _, callErr := chatPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, 0x00020002)
	if played == 0 {
		slog.Debug("play chat notification sound", "error", callErr)
	}
}
