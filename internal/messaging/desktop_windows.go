package messaging

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
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
			index := int32(-20)
			style, _, _ := chatGetStyle.Call(hwnd, uintptr(index))
			chatSetStyle.Call(hwnd, uintptr(index), style|0x08000000)
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
	// worker; the sound lasts only 360ms and needs no external executable.
	chatPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, 0x00020002)
}
