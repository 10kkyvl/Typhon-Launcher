//go:build windows

package instance

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

func TestClaimGoesToTheFirstCaller(t *testing.T) {
	var sys windowsAPI
	for i, want := range []bool{true, false} {
		got, err := sys.Claim("typhon-claim-test-early")
		if err != nil || got != want {
			t.Fatalf("claim %d = %v, %v; want %v", i, got, err, want)
		}
	}
}

func sendToReceiver(id string, args ...string) error {
	var sys windowsAPI
	name := earlyPrefix + id + handoffSuffix
	window, err := sys.FindWindow(name, name)
	if err != nil {
		return err
	}
	if window == 0 {
		return errors.New("receiver window not found")
	}
	payload, err := json.Marshal(message{Args: args})
	if err != nil {
		return err
	}
	return sys.Send(window, copyDataID, string(payload))
}

func TestListenTakesABurstWhileTheMainThreadPumpsNothing(t *testing.T) {
	const id = "typhon.listen.burst"
	var mu sync.Mutex
	var got []string
	l, err := Listen(t.Context(), id, func(args []string) {
		mu.Lock()
		got = append(got, args[0])
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	for i := range 10 {
		if err := sendToReceiver(id, strconv.Itoa(i)); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sendToReceiver(id, "p"+strconv.Itoa(i)); err != nil {
				t.Errorf("parallel send %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 20 {
		t.Fatalf("received %d of 20: %v", len(got), got)
	}
	for i := range 10 {
		if got[i] != strconv.Itoa(i) {
			t.Fatalf("order lost: %v", got[:10])
		}
	}
}

func TestListenStopsWithoutLeakingGoroutines(t *testing.T) {
	const id = "typhon.listen.stop"
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(t.Context())
	l, err := Listen(ctx, id, func([]string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(t.Context(), id, func([]string) {}); !errors.Is(err, ErrAlreadyListening) {
		t.Fatalf("second Listen err = %v", err)
	}
	cancel()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sendToReceiver(id, "late"); err == nil {
		t.Fatal("the receiver window survived Close")
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines: %d before, %d after", before, after)
	}
}

func TestForwardReachesTheReceiverOfAFirstInstance(t *testing.T) {
	const id = "typhon.listen.forward"
	var sys windowsAPI
	if first, err := sys.Claim(earlyPrefix + id + earlySuffix); err != nil || !first {
		t.Fatalf("claim = %v, %v", first, err)
	}
	got := make(chan []string, 1)
	l, err := Listen(t.Context(), id, func(args []string) { got <- args })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	forwarded, err := forward(t.Context(), sys, sleep, id, []string{"typhon.exe", "--open", "library"}, "")
	if err != nil || !forwarded {
		t.Fatalf("forward = %v, %v", forwarded, err)
	}
	if args := <-got; len(args) != 3 || args[2] != "library" {
		t.Fatalf("args = %v", args)
	}
}
