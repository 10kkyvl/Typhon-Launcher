//go:build windows

package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
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

const (
	wmQuit                  = 0x0012
	errorClassAlreadyExists = windows.Errno(1410)
)

var (
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procUnregisterClass   = user32.NewProc("UnregisterClassW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procGetMessage        = user32.NewProc("GetMessageW")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")

	handoffProc = syscall.NewCallback(handoffWndProc)
	active      atomic.Pointer[dispatcher]
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

type winMsg struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

func handoffWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	if msg == wmCopyData {
		d := active.Load()
		//nolint:gosec // G103: lparam — COPYDATASTRUCT отправителя, SendMessage держит его живым до возврата оконной процедуры.
		cds := (*copyData)(*(*unsafe.Pointer)(unsafe.Pointer(&lparam)))
		if d == nil || cds.data != copyDataID {
			return 0
		}
		//nolint:gosec // G103: buffer — строка UTF-16 отправителя с нулевым терминатором, живая до возврата SendMessage.
		text := windows.UTF16PtrToString((*uint16)(*(*unsafe.Pointer)(unsafe.Pointer(&cds.buffer))))
		var m message
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			return 0
		}
		d.push(m.Args)
		return 1
	}
	//nolint:errcheck // третий результат Call — GetLastError, а не признак сбоя.
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

type winListener struct {
	d          *dispatcher
	thread     uint32
	threadDone chan struct{}
	closed     chan struct{}
	watchDone  chan struct{}
	once       sync.Once
	err        error
}

type pumpReady struct {
	thread uint32
	err    error
}

// Messages sent to a window are handled only by the thread that owns it, so
// the window lives on a thread of its own and the first instance's main thread,
// which pumps nothing until its services are up, is not involved.
func Listen(ctx context.Context, id string, sink func(args []string)) (Listener, error) {
	if id == "" {
		return nil, ErrNoID
	}
	d := newDispatcher(sink)
	if !active.CompareAndSwap(nil, d) {
		d.close()
		return nil, ErrAlreadyListening
	}
	l := &winListener{d: d, threadDone: make(chan struct{}), closed: make(chan struct{}), watchDone: make(chan struct{})}
	ready := make(chan pumpReady, 1)
	go l.pump(earlyPrefix+id+handoffSuffix, ready)
	r := <-ready
	if r.err != nil {
		<-l.threadDone
		active.Store(nil)
		d.close()
		return nil, r.err
	}
	l.thread = r.thread
	go func() {
		defer close(l.watchDone)
		select {
		case <-ctx.Done():
			l.shutdown()
		case <-l.closed:
		}
	}()
	return l, nil
}

func (l *winListener) pump(name string, ready chan<- pumpReady) {
	defer close(l.threadDone)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var inst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &inst); err != nil {
		ready <- pumpReady{err: fmt.Errorf("module handle: %w", err)}
		return
	}
	className, err := windows.UTF16PtrFromString(name)
	if err != nil {
		ready <- pumpReady{err: err}
		return
	}
	class := wndClass{proc: handoffProc, instance: inst, className: className}
	class.size = uint32(unsafe.Sizeof(class))
	//nolint:gosec // G103: WNDCLASSEX читается синхронно внутри RegisterClassExW.
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); r == 0 && !errors.Is(err, errorClassAlreadyExists) {
		ready <- pumpReady{err: fmt.Errorf("register window class: %w", err)}
		return
	}
	defer func() {
		//nolint:gosec,errcheck // G103: указатель на имя класса живёт до возврата; неудача отмены регистрации при выходе потока безвредна, класс уйдёт с процессом.
		procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), uintptr(inst))
	}()
	//nolint:gosec // G103: указатели на строки живут до возврата CreateWindowExW.
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, hwndMessage, 0, uintptr(inst), 0)
	if hwnd == 0 {
		ready <- pumpReady{err: fmt.Errorf("create receiver window: %w", err)}
		return
	}
	ready <- pumpReady{thread: windows.GetCurrentThreadId()}
	var m winMsg
	for {
		//nolint:errcheck,gosec // G103: MSG живёт на стеке потока; третий результат Call — GetLastError, признак конца цикла — r.
		if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); r == 0 || r == ^uintptr(0) {
			break
		}
		//nolint:errcheck,gosec // G103: MSG живёт на стеке потока; G104: результат DispatchMessage — ответ оконной процедуры, а не ошибка.
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	if r, _, err := procDestroyWindow.Call(hwnd); r == 0 {
		slog.Warn("destroy second launch receiver window", "error", err)
	}
}

func (l *winListener) shutdown() {
	l.once.Do(func() {
		if r, _, err := procPostThreadMessage.Call(uintptr(l.thread), wmQuit, 0, 0); r == 0 {
			l.err = fmt.Errorf("stop the second launch receiver: %w", err)
			return
		}
		<-l.threadDone
		active.Store(nil)
		l.d.close()
		close(l.closed)
	})
}

func (l *winListener) Close() error {
	l.shutdown()
	select {
	case <-l.closed:
		<-l.watchDone
	default:
	}
	return l.err
}
