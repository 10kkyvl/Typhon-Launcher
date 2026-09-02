//go:build windows

package install

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
)

const (
	sFalse            = syscall.Errno(1)
	rpcErrChangedMode = syscall.Errno(0x80010106)
)

var (
	modshell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = modshell32.NewProc("ShellExecuteExW")
)

var errShellExecute = errors.New("ShellExecuteEx завершился с ошибкой")

type shellExecuteInfo struct {
	Size          uint32
	Mask          uint32
	Hwnd          windows.HWND
	Verb          *uint16
	File          *uint16
	Parameters    *uint16
	Directory     *uint16
	Show          int32
	InstApp       windows.Handle
	IDList        uintptr
	Class         *uint16
	KeyClass      windows.Handle
	HotKey        uint32
	IconOrMonitor windows.Handle
	Process       windows.Handle
}

// CreateProcess никогда не поднимает UAC: для установщика с requireAdministrator
// в манифесте он возвращает ERROR_ELEVATION_REQUIRED, и запросить права можно
// только через ShellExecuteEx с глаголом runas.
func needsElevation(err error) bool {
	return errors.Is(err, windows.ERROR_ELEVATION_REQUIRED)
}

// Отказ в окне UAC приходит как ERROR_CANCELLED и означает решение
// пользователя, а не сбой установки.
func elevationError(path string, err error) error {
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return errElevationDeclined
	}
	return fmt.Errorf("запуск установщика %s с правами администратора: %w", path, err)
}

type elevatedProc struct {
	mu     sync.Mutex
	handle windows.Handle
}

func (p *elevatedProc) terminate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return nil
	}
	return windows.TerminateProcess(p.handle, 1)
}

func (p *elevatedProc) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return
	}
	if err := windows.CloseHandle(p.handle); err != nil {
		slog.Warn("close elevated process handle", "error", err)
	}
	p.handle = 0
}

func (p *elevatedProc) wait() (int, error) {
	p.mu.Lock()
	handle := p.handle
	p.mu.Unlock()
	return awaitProcess(handle)
}

func awaitProcess(handle windows.Handle) (int, error) {
	event, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return 0, fmt.Errorf("ожидание установщика: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("ожидание установщика: код %d", event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return 0, fmt.Errorf("код возврата установщика: %w", err)
	}
	return int(code), nil
}

func workerStartError(path string, err error) error {
	return elevationError(path, err)
}

func elevationParams(spec runSpec) string {
	if spec.Tail != "" {
		return spec.Tail
	}
	parts := make([]string, 0, len(spec.Args))
	for _, arg := range spec.Args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	return strings.Join(parts, " ")
}

func startElevated(spec runSpec) (workerHandle, error) {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return nil, fmt.Errorf("глагол runas: %w", err)
	}
	file, err := windows.UTF16PtrFromString(spec.Path)
	if err != nil {
		return nil, fmt.Errorf("путь установщика %s: %w", spec.Path, err)
	}
	var params *uint16
	if tail := elevationParams(spec); tail != "" {
		params, err = windows.UTF16PtrFromString(tail)
		if err != nil {
			return nil, fmt.Errorf("аргументы установщика %s: %w", tail, err)
		}
	}
	var dir *uint16
	if spec.Dir != "" {
		dir, err = windows.UTF16PtrFromString(spec.Dir)
		if err != nil {
			return nil, fmt.Errorf("рабочий каталог %s: %w", spec.Dir, err)
		}
	}
	show := int32(windows.SW_SHOWNORMAL)
	if spec.Hidden {
		show = int32(windows.SW_HIDE)
	}
	info := &shellExecuteInfo{
		//nolint:gosec // G115: размер собственной структуры в uint32 помещается
		Size:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		Mask:       seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		Verb:       verb,
		File:       file,
		Parameters: params,
		Directory:  dir,
		Show:       show,
	}
	if err := shellExecuteEx(info); err != nil {
		return nil, err
	}
	if info.Process == 0 {
		return nil, errNoElevatedProcess
	}
	return &elevatedProc{handle: info.Process}, nil
}

// ShellExecuteEx уходит в оболочку и её расширения, поэтому вызывается на
// отдельном потоке с COM в однопоточной апартаменте; SEE_MASK_NOASYNC держит
// вызов синхронным, иначе поток нельзя было бы отпускать.
func shellExecuteEx(info *shellExecuteInfo) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		owned, err := initCOM()
		if err != nil {
			done <- err
			return
		}
		if owned {
			defer windows.CoUninitialize()
		}
		done <- callShellExecuteEx(info)
	}()
	return <-done
}

func callShellExecuteEx(info *shellExecuteInfo) error {
	if err := procShellExecuteExW.Find(); err != nil {
		return fmt.Errorf("shell32.ShellExecuteExW: %w", err)
	}
	//nolint:gosec // G103: ShellExecuteExW принимает SHELLEXECUTEINFOW только по указателю
	ptr := uintptr(unsafe.Pointer(info))
	r1, _, callErr := syscall.SyscallN(procShellExecuteExW.Addr(), ptr)
	runtime.KeepAlive(info)
	if r1 != 0 {
		return nil
	}
	if callErr != 0 {
		return callErr
	}
	return errShellExecute
}

func initCOM() (bool, error) {
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	switch {
	case err == nil, errors.Is(err, sFalse):
		return true, nil
	case errors.Is(err, rpcErrChangedMode):
		return false, nil
	default:
		return false, fmt.Errorf("com init: %w", err)
	}
}
