//go:build windows

package instance

import (
	"encoding/json"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type wndClass struct {
	size       uint32
	style      uint32
	proc       uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type received struct {
	id      uintptr
	payload string
}

func TestForwardDeliversToAWindowLikeWails(t *testing.T) {
	const id = "typhon.instance.test"
	prefix := namePrefix + id
	className := prefix + classSuffix
	got := make(chan received, 1)
	ready := make(chan error, 1)
	threads := make(chan uint32, 1)
	done := make(chan struct{})
	defProc := user32.NewProc("DefWindowProcW")

	proc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
		if msg == wmCopyData {
			//nolint:gosec // G103: lparam — COPYDATASTRUCT, который SendMessage держит живым на время обработки.
			cds := (*copyData)(*(*unsafe.Pointer)(unsafe.Pointer(&lparam)))
			//nolint:gosec // G103: buffer — строка UTF-16 отправителя, живая до возврата SendMessage.
			text := windows.UTF16PtrToString((*uint16)(*(*unsafe.Pointer)(unsafe.Pointer(&cds.buffer))))
			got <- received{id: cds.data, payload: text}
			return 0
		}
		//nolint:errcheck // третий результат Call — GetLastError, а не признак сбоя.
		r, _, _ := defProc.Call(hwnd, msg, wparam, lparam)
		return r
	})

	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(prefix+mutexSuffix))
		if err != nil {
			ready <- err
			return
		}
		defer func() {
			if err := windows.CloseHandle(mutex); err != nil {
				t.Errorf("close mutex: %v", err)
			}
		}()
		var inst windows.Handle
		if err := windows.GetModuleHandleEx(0, nil, &inst); err != nil {
			ready <- err
			return
		}
		class := wndClass{proc: proc, instance: inst, className: windows.StringToUTF16Ptr(className)}
		class.size = uint32(unsafe.Sizeof(class))
		//nolint:gosec // G103: WNDCLASSEX читается синхронно внутри RegisterClassExW.
		if r, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); r == 0 {
			ready <- err
			return
		}
		//nolint:gosec // G103: указатели на строки живут до возврата CreateWindowExW.
		hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0,
			uintptr(unsafe.Pointer(class.className)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(prefix+windowSuffix))),
			0, 0, 0, 0, 0, hwndMessage, 0, uintptr(inst), 0)
		if hwnd == 0 {
			ready <- err
			return
		}
		threads <- windows.GetCurrentThreadId()
		ready <- nil
		var m struct {
			hwnd    uintptr
			message uint32
			wparam  uintptr
			lparam  uintptr
			time    uint32
			ptX     int32
			ptY     int32
		}
		getMessage := user32.NewProc("GetMessageW")
		dispatch := user32.NewProc("DispatchMessageW")
		for {
			//nolint:errcheck,gosec // G103: MSG живёт на стеке горутины; третий результат Call — GetLastError, признак конца цикла — r.
			if r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); r == 0 || r == ^uintptr(0) {
				break
			}
			dispatch.Call(uintptr(unsafe.Pointer(&m))) //nolint:errcheck,gosec // G104: результат DispatchMessage — ответ оконной процедуры, а не ошибка
		}
		if r, _, err := user32.NewProc("DestroyWindow").Call(hwnd); r == 0 {
			t.Errorf("destroy window: %v", err)
		}
	}()
	if err := <-ready; err != nil {
		t.Fatalf("start the receiver window: %v", err)
	}
	thread := <-threads
	defer func() {
		if r, _, err := user32.NewProc("PostThreadMessageW").Call(uintptr(thread), 0x0012, 0, 0); r == 0 {
			t.Errorf("stop the receiver: %v", err)
		}
		<-done
	}()

	args := []string{`C:\typhon.exe`, "--open", "downloads"}
	forwarded, err := Forward(t.Context(), id, args, `C:\work`)
	if err != nil || !forwarded {
		t.Fatalf("Forward = %v, %v", forwarded, err)
	}
	r := <-got
	if r.id != copyDataID {
		t.Fatalf("data id = %d", r.id)
	}
	var data struct {
		Args       []string `json:"args"`
		WorkingDir string   `json:"workingDir"`
	}
	if err := json.Unmarshal([]byte(r.payload), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Args) != 3 || data.Args[2] != "downloads" || data.WorkingDir != `C:\work` {
		t.Fatalf("payload = %s", r.payload)
	}
}

func TestForwardWithoutFirstInstance(t *testing.T) {
	forwarded, err := Forward(t.Context(), "typhon.instance.absent", []string{"typhon.exe"}, "")
	if err != nil || forwarded {
		t.Fatalf("Forward = %v, %v", forwarded, err)
	}
}
