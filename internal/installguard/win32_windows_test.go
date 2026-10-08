//go:build windows

package installguard

import (
	"os"
	"os/exec"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	createWindowEx    = user32.NewProc("CreateWindowExW")
	destroyWindow     = user32.NewProc("DestroyWindow")
	registerClassEx   = user32.NewProc("RegisterClassExW")
	unregisterClass   = user32.NewProc("UnregisterClassW")
	isWindow          = user32.NewProc("IsWindow")
	showWindowSync    = user32.NewProc("ShowWindow")
	getMessage        = user32.NewProc("GetMessageW")
	translateMessage  = user32.NewProc("TranslateMessage")
	dispatchMessage   = user32.NewProc("DispatchMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
)

func win32(proc *windows.LazyProc, args ...uintptr) uintptr {
	//nolint:errcheck // главный принцип: исход вызова проверяется состоянием окна или процесса в самом тесте, last-error здесь не авторитетен
	r1, _, _ := proc.Call(args...)
	return r1
}

func msgCall(proc *windows.LazyProc, msg *fixtureMessage) uintptr {
	//nolint:errcheck,gosec // главный принцип: адрес сообщения уходит только в синхронный Win32-вызов, исход проверяет вызывающий по возвращаемому значению
	r1, _, _ := proc.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
	return r1
}

func reexec(mode string) *exec.Cmd {
	//nolint:gosec // инвариант 32: внешнего ввода нет, тестовый бинарь перезапускает сам себя как стенд-ин
	cmd := exec.Command(os.Args[0], "-test.run=^TestVerifierHelperProcess$")
	cmd.Env = append(os.Environ(), verifierHelperEnv+"="+mode)
	return cmd
}

func createWindow(t *testing.T, class, title string) uintptr {
	t.Helper()
	name := windows.StringToUTF16Ptr(class)
	caption := windows.StringToUTF16Ptr(title)
	//nolint:gosec // главный принцип: адреса живых буферов уходят в синхронный Win32-вызов, а ошибка вызова доходит до теста
	top, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(caption)), 0x10CF0000, 0, 0, 400, 200, 0, 0, 0, 0)
	if top == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { win32(destroyWindow, top) })
	return top
}

func registerWindowClass(t *testing.T, class string, proc uintptr) {
	t.Helper()
	wc := &fixtureClass{Proc: proc, Name: windows.StringToUTF16Ptr(class)}
	wc.Size = uint32(unsafe.Sizeof(*wc))
	//nolint:gosec // главный принцип: адрес живой структуры уходит в синхронный Win32-вызов, а ошибка вызова доходит до теста
	atom, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(wc)))
	if atom == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:errcheck,gosec // главный принцип: класс держит окно, и следующий RegisterClassEx сам сообщит, если снятие не удалось
		unregisterClass.Call(uintptr(unsafe.Pointer(wc.Name)), 0)
	})
}

func windowAlive(top uintptr) bool {
	return win32(isWindow, top) != 0
}

// ShowWindowAsync needs a message pump and lands at a load-dependent moment, so
// tests swap in this synchronous call for windows they own.
func showSync(top, command uintptr) {
	win32(showWindowSync, top, command)
}
