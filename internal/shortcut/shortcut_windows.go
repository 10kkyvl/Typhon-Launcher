//go:build windows

package shortcut

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Supported() bool { return true }

func DesktopDir() (string, error) {
	path, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("shortcut: известная папка Desktop: %w", err)
	}
	return path, nil
}

const (
	sFalse            = syscall.Errno(0x00000001)
	rpcErrChangedMode = syscall.Errno(0x80010106)
)

var (
	clsidShellLink = windows.GUID{
		Data1: 0x00021401,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidShellLinkW = windows.GUID{
		Data1: 0x000214F9,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidPersistFile = windows.GUID{
		Data1: 0x0000010B,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
)

var (
	modole32             = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = modole32.NewProc("CoCreateInstance")
)

// IShellLinkW не входит в golang.org/x/sys/windows (только CoInitializeEx/
// CoUninitialize), поэтому вызывается вручную по vtable через CoCreateInstance
// из ole32.dll.
type shellLinkVtbl struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	GetPath             uintptr
	GetIDList           uintptr
	SetIDList           uintptr
	GetDescription      uintptr
	SetDescription      uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments        uintptr
	SetArguments        uintptr
	GetHotkey           uintptr
	SetHotkey           uintptr
	GetShowCmd          uintptr
	SetShowCmd          uintptr
	GetIconLocation     uintptr
	SetIconLocation     uintptr
	SetRelativePath     uintptr
	Resolve             uintptr
	SetPath             uintptr
}

type shellLink struct {
	vtbl *shellLinkVtbl
}

type persistFileVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetClassID     uintptr
	IsDirty        uintptr
	Load           uintptr
	Save           uintptr
	SaveCompleted  uintptr
	GetCurFile     uintptr
}

type persistFile struct {
	vtbl *persistFileVtbl
}

// callMethod invokes a COM vtable method (or a raw WinAPI function such as
// CoCreateInstance) by its function-pointer address. COM signals failure
// through the returned HRESULT/refcount, never through GetLastError, so the
// syscall.Errno SyscallN also returns is always stale here.
//
//nolint:errcheck // GetLastError is not how COM/CoCreateInstance report failure; status is the returned r0.
func callMethod(method uintptr, args ...uintptr) uintptr {
	r0, _, _ := syscall.SyscallN(method, args...)
	return r0
}

func Create(path string, link Link) error {
	if path == "" {
		return errors.New("shortcut: путь ярлыка пуст")
	}
	if link.Target == "" {
		return errors.New("shortcut: цель ярлыка (Target) пуста")
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("shortcut: каталог назначения %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("shortcut: %s не является каталогом", dir)
	}

	// COM-апартамент привязан к потоку ОС: без LockOSThread рантайм Go может
	// переставить горутину на другой поток посреди вызовов.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInitialize(); err != nil {
		return err
	}
	defer windows.CoUninitialize()

	sl, err := createShellLink()
	if err != nil {
		return err
	}
	defer sl.release()

	if err := sl.setPath(link.Target); err != nil {
		return err
	}
	if err := sl.setArguments(link.Args); err != nil {
		return err
	}
	if err := sl.setDescription(link.Description); err != nil {
		return err
	}
	if link.WorkDir != "" {
		if err := sl.setWorkingDirectory(link.WorkDir); err != nil {
			return err
		}
	}
	if link.Icon != "" {
		if err := sl.setIconLocation(link.Icon); err != nil {
			return err
		}
	}

	pf, err := sl.queryPersistFile()
	if err != nil {
		return err
	}
	defer pf.release()

	return pf.save(path)
}

func coInitialize() error {
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sFalse):
		// S_FALSE: COM уже инициализирован на этом потоке в совместимом
		// режиме — не ошибка, но CoUninitialize всё равно должен быть
		// вызван парно с этим Init.
		return nil
	case errors.Is(err, rpcErrChangedMode):
		return fmt.Errorf("shortcut: CoInitializeEx: поток уже в апартаменте другого режима: %w", err)
	default:
		return fmt.Errorf("shortcut: CoInitializeEx: %w", err)
	}
}

// createShellLink activates IShellLinkW through CoCreateInstance. The GUID
// and output-pointer addresses taken here all point at values that outlive
// this call (a package-level var or a local pinned by the call itself), and
// the resulting HRESULT (32 bits, in r0's low half) is what callMethod's
// callers check.
//
//nolint:gosec // G103: addresses of the CLSID/IID and the output pointer, all live for the call. G115: HRESULT is the 32-bit value in r0.
func createShellLink() (*shellLink, error) {
	if err := procCoCreateInstance.Find(); err != nil {
		return nil, fmt.Errorf("shortcut: ole32.dll!CoCreateInstance: %w", err)
	}
	var obj unsafe.Pointer
	r0 := callMethod(
		procCoCreateInstance.Addr(),
		uintptr(unsafe.Pointer(&clsidShellLink)),
		0,
		uintptr(windows.CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(&iidShellLinkW)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr := int32(r0); hr < 0 {
		return nil, hresultErr("CoCreateInstance(IShellLinkW)", hr)
	}
	if obj == nil {
		return nil, errors.New("shortcut: CoCreateInstance(IShellLinkW) вернул нулевой указатель при успешном HRESULT")
	}
	return (*shellLink)(obj), nil
}

//nolint:gosec // G115: hr is already the truncated 32-bit HRESULT; formatting it as uint32 does not overflow.
func hresultErr(call string, hr int32) error {
	return fmt.Errorf("shortcut: %s: hresult 0x%08X", call, uint32(hr))
}

//nolint:gosec // G103: address of the receiver, which is the COM object itself and outlives this call.
func (s *shellLink) release() {
	callMethod(s.vtbl.Release, uintptr(unsafe.Pointer(s)))
}

// setString calls a single-string IShellLinkW setter (SetPath, SetArguments,
// SetDescription, SetWorkingDirectory). All of them share the signature
// HRESULT SetX(LPCWSTR).
//
//nolint:gosec // G103: addresses of the receiver and of the UTF-16 buffer, both live for the call. G115: HRESULT is the 32-bit value in r0.
func (s *shellLink) setString(method uintptr, call, value string) error {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return fmt.Errorf("shortcut: %s: кодирование %q: %w", call, value, err)
	}
	r0 := callMethod(method, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(ptr)))
	if hr := int32(r0); hr < 0 {
		return hresultErr(call, hr)
	}
	return nil
}

func (s *shellLink) setPath(v string) error {
	return s.setString(s.vtbl.SetPath, "SetPath", v)
}

func (s *shellLink) setArguments(v string) error {
	return s.setString(s.vtbl.SetArguments, "SetArguments", v)
}

func (s *shellLink) setWorkingDirectory(v string) error {
	return s.setString(s.vtbl.SetWorkingDirectory, "SetWorkingDirectory", v)
}

func (s *shellLink) setDescription(v string) error {
	return s.setString(s.vtbl.SetDescription, "SetDescription", v)
}

//nolint:gosec // G103: addresses of the receiver and of the UTF-16 buffer, both live for the call. G115: HRESULT is the 32-bit value in r0.
func (s *shellLink) setIconLocation(v string) error {
	ptr, err := windows.UTF16PtrFromString(v)
	if err != nil {
		return fmt.Errorf("shortcut: SetIconLocation: кодирование %q: %w", v, err)
	}
	r0 := callMethod(s.vtbl.SetIconLocation, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(ptr)), 0)
	if hr := int32(r0); hr < 0 {
		return hresultErr("SetIconLocation", hr)
	}
	return nil
}

//nolint:gosec // G103: addresses of the receiver, the target IID, and the output pointer, all live for the call. G115: HRESULT is the 32-bit value in r0.
func (s *shellLink) queryPersistFile() (*persistFile, error) {
	var obj unsafe.Pointer
	r0 := callMethod(
		s.vtbl.QueryInterface,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&iidPersistFile)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr := int32(r0); hr < 0 {
		return nil, hresultErr("QueryInterface(IPersistFile)", hr)
	}
	if obj == nil {
		return nil, errors.New("shortcut: QueryInterface(IPersistFile) вернул нулевой указатель при успешном HRESULT")
	}
	return (*persistFile)(obj), nil
}

//nolint:gosec // G103: address of the receiver, which is the COM object itself and outlives this call.
func (p *persistFile) release() {
	callMethod(p.vtbl.Release, uintptr(unsafe.Pointer(p)))
}

//nolint:gosec // G103: addresses of the receiver and of the UTF-16 path buffer, both live for the call. G115: HRESULT is the 32-bit value in r0.
func (p *persistFile) save(path string) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("shortcut: Save: кодирование пути %q: %w", path, err)
	}
	r0 := callMethod(p.vtbl.Save, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(ptr)), 1)
	if hr := int32(r0); hr < 0 {
		return hresultErr("Save", hr)
	}
	return nil
}
