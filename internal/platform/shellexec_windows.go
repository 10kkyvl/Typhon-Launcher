package platform

import (
	"errors"
	"fmt"
	"runtime"
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
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
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

// Хэндл запущенного процесса принадлежит вызывающему. Нулевой хэндл без
// ошибки возможен, когда оболочка передала запрос уже работающему процессу,
// а не стартовала новый.
func ShellExecute(verb, file, params, dir string, show int32) (windows.Handle, error) {
	verbPtr, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return 0, fmt.Errorf("глагол %s: %w", verb, err)
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, fmt.Errorf("путь %s: %w", file, err)
	}
	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return 0, fmt.Errorf("аргументы %s: %w", params, err)
		}
	}
	var dirPtr *uint16
	if dir != "" {
		dirPtr, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return 0, fmt.Errorf("рабочий каталог %s: %w", dir, err)
		}
	}
	info := &shellExecuteInfo{
		//nolint:gosec // G115: размер собственной структуры в uint32 помещается
		Size:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		Mask:       seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		Verb:       verbPtr,
		File:       filePtr,
		Parameters: paramsPtr,
		Directory:  dirPtr,
		Show:       show,
	}
	if err := shellExecuteEx(info); err != nil {
		return 0, err
	}
	return info.Process, nil
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
