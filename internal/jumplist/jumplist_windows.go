//go:build windows

package jumplist

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Supported() bool { return true }

const (
	sFalse            = syscall.Errno(0x00000001)
	rpcErrChangedMode = syscall.Errno(0x80010106)
	eNoInterface      = int32(-0x7FFFBFFE) // 0x80004002
	vtLPWSTR          = 31
	argsBufLen        = 2048
)

var (
	clsidDestinationList = windows.GUID{
		Data1: 0x77F10CF0, Data2: 0x3DB5, Data3: 0x4966,
		Data4: [8]byte{0xB5, 0x20, 0xB7, 0xC5, 0x4F, 0xD3, 0x5E, 0xD6},
	}
	iidCustomDestinationList = windows.GUID{
		Data1: 0x6332DEBF, Data2: 0x87B5, Data3: 0x4670,
		Data4: [8]byte{0x90, 0xC0, 0x5E, 0x57, 0xB4, 0x08, 0xA4, 0x9E},
	}
	clsidEnumerableObjectCollection = windows.GUID{
		Data1: 0x2D3468C1, Data2: 0x36A7, Data3: 0x43B6,
		Data4: [8]byte{0xAC, 0x24, 0xD3, 0xF0, 0x2F, 0xD9, 0x60, 0x7A},
	}
	iidObjectCollection = windows.GUID{
		Data1: 0x5632B1A4, Data2: 0xE38A, Data3: 0x400A,
		Data4: [8]byte{0x92, 0x8A, 0xD4, 0xCD, 0x63, 0x23, 0x02, 0x95},
	}
	iidObjectArray = windows.GUID{
		Data1: 0x92CA9DCD, Data2: 0x5622, Data3: 0x4BBA,
		Data4: [8]byte{0xA8, 0x05, 0x5E, 0x9F, 0x54, 0x1B, 0xD8, 0xC9},
	}
	clsidShellLink = windows.GUID{
		Data1: 0x00021401,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidShellLinkW = windows.GUID{
		Data1: 0x000214F9,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidPropertyStore = windows.GUID{
		Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446,
		Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99},
	}
	pkeyTitle = propertyKey{
		fmtid: windows.GUID{
			Data1: 0xF29F85E0, Data2: 0x4FF9, Data3: 0x1068,
			Data4: [8]byte{0xAB, 0x91, 0x08, 0x00, 0x2B, 0x27, 0xB3, 0xD9},
		},
		pid: 2,
	}
)

var (
	modole32             = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = modole32.NewProc("CoCreateInstance")
)

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

// PROPVARIANT layout: 8 header bytes and a 16-byte union on 64-bit.
type propVariant struct {
	vt   uint16
	_    [3]uint16
	data [2]unsafe.Pointer
}

type unknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type unknown struct {
	vtbl *unknownVtbl
}

type customDestListVtbl struct {
	unknownVtbl
	SetAppID               uintptr
	BeginList              uintptr
	AppendCategory         uintptr
	AppendKnownCategory    uintptr
	AddUserTasks           uintptr
	CommitList             uintptr
	GetRemovedDestinations uintptr
	DeleteList             uintptr
	AbortList              uintptr
}

type destinationList struct {
	vtbl *customDestListVtbl
}

type objectArrayVtbl struct {
	unknownVtbl
	GetCount uintptr
	GetAt    uintptr
}

type objectArray struct {
	vtbl *objectArrayVtbl
}

type objectCollectionVtbl struct {
	objectArrayVtbl
	AddObject      uintptr
	AddFromArray   uintptr
	RemoveObjectAt uintptr
	Clear          uintptr
}

type objectCollection struct {
	vtbl *objectCollectionVtbl
}

type shellLinkVtbl struct {
	unknownVtbl
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

type propertyStoreVtbl struct {
	unknownVtbl
	GetCount uintptr
	GetAt    uintptr
	GetValue uintptr
	SetValue uintptr
	Commit   uintptr
}

type propertyStore struct {
	vtbl *propertyStoreVtbl
}

// callMethod invokes a COM vtable method by its function-pointer address. COM
// reports failure through the returned HRESULT, never through GetLastError, so
// the syscall.Errno SyscallN also returns is stale here.
//
//nolint:errcheck // GetLastError is not how COM reports failure; status is the returned r0.
func callMethod(method uintptr, args ...uintptr) uintptr {
	r0, _, _ := syscall.SyscallN(method, args...)
	return r0
}

//nolint:gosec // G115: hr is already the truncated 32-bit HRESULT; formatting it as uint32 does not overflow.
func hresultErr(call string, hr int32) error {
	return fmt.Errorf("jumplist: %s: hresult 0x%08X", call, uint32(hr))
}

//nolint:gosec // G115: HRESULT is the 32-bit value in r0.
func hresult(r0 uintptr) int32 { return int32(r0) }

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
		return fmt.Errorf("jumplist: CoInitializeEx: поток уже в апартаменте другого режима: %w", err)
	default:
		return fmt.Errorf("jumplist: CoInitializeEx: %w", err)
	}
}

//nolint:gosec // G103: addresses of the CLSID/IID and the output pointer, all live for the call.
func coCreate(clsid, iid *windows.GUID, name string) (unsafe.Pointer, error) {
	if err := procCoCreateInstance.Find(); err != nil {
		return nil, fmt.Errorf("jumplist: ole32.dll!CoCreateInstance: %w", err)
	}
	var obj unsafe.Pointer
	r0 := callMethod(
		procCoCreateInstance.Addr(),
		uintptr(unsafe.Pointer(clsid)),
		0,
		uintptr(windows.CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr := hresult(r0); hr < 0 {
		return nil, hresultErr("CoCreateInstance("+name+")", hr)
	}
	if obj == nil {
		return nil, fmt.Errorf("jumplist: CoCreateInstance(%s) вернул нулевой указатель при успешном HRESULT", name)
	}
	return obj, nil
}

// queryInterface returns the raw HRESULT next to the error so a caller can
// tell "this object is not that kind" from a real failure.
//
//nolint:gosec // G103: addresses of the receiver, the IID and the output pointer, all live for the call.
func queryInterface(obj unsafe.Pointer, iid *windows.GUID, name string) (unsafe.Pointer, int32, error) {
	var out unsafe.Pointer
	u := (*unknown)(obj)
	r0 := callMethod(u.vtbl.QueryInterface, uintptr(obj), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	hr := hresult(r0)
	if hr < 0 {
		return nil, hr, hresultErr("QueryInterface("+name+")", hr)
	}
	if out == nil {
		return nil, hr, fmt.Errorf("jumplist: QueryInterface(%s) вернул нулевой указатель при успешном HRESULT", name)
	}
	return out, hr, nil
}

//nolint:gosec // G103: address of the receiver, which is the COM object itself and outlives this call.
func release(obj unsafe.Pointer) {
	callMethod((*unknown)(obj).vtbl.Release, uintptr(obj))
}

//nolint:gosec // G103: addresses of the receiver and of the UTF-16 buffer, both live for the call.
func callWithString(method uintptr, obj unsafe.Pointer, call, value string) error {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return fmt.Errorf("jumplist: %s: кодирование %q: %w", call, value, err)
	}
	r0 := callMethod(method, uintptr(obj), uintptr(unsafe.Pointer(ptr)))
	runtime.KeepAlive(ptr)
	if hr := hresult(r0); hr < 0 {
		return hresultErr(call, hr)
	}
	return nil
}

func callPlain(method uintptr, obj unsafe.Pointer, call string) error {
	if hr := hresult(callMethod(method, uintptr(obj))); hr < 0 {
		return hresultErr(call, hr)
	}
	return nil
}

func createDestinationList() (*destinationList, error) {
	obj, err := coCreate(&clsidDestinationList, &iidCustomDestinationList, "DestinationList")
	if err != nil {
		return nil, err
	}
	return (*destinationList)(obj), nil
}

//nolint:gosec // G103: pointer to the COM object itself.
func (l *destinationList) raw() unsafe.Pointer { return unsafe.Pointer(l) }

func (l *destinationList) setAppID(id string) error {
	return callWithString(l.vtbl.SetAppID, l.raw(), "SetAppID", id)
}

// beginList hands back the destinations the user removed since the last
// commit. They must stay out of the next list.
//
//nolint:gosec // G103: addresses of the receiver, the min-slots counter, the IID and the output pointer, all live for the call.
func (l *destinationList) beginList() (*objectArray, error) {
	var slots uint32
	var obj unsafe.Pointer
	r0 := callMethod(
		l.vtbl.BeginList,
		uintptr(l.raw()),
		uintptr(unsafe.Pointer(&slots)),
		uintptr(unsafe.Pointer(&iidObjectArray)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr := hresult(r0); hr < 0 {
		return nil, hresultErr("BeginList", hr)
	}
	if obj == nil {
		return nil, errors.New("jumplist: BeginList вернул нулевой указатель при успешном HRESULT")
	}
	return (*objectArray)(obj), nil
}

//nolint:gosec // G103: addresses of the receiver, the UTF-16 buffer and the collection, all live for the call.
func (l *destinationList) appendCategory(title string, items *objectCollection) error {
	ptr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return fmt.Errorf("jumplist: AppendCategory: кодирование %q: %w", title, err)
	}
	r0 := callMethod(l.vtbl.AppendCategory, uintptr(l.raw()), uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(items)))
	runtime.KeepAlive(ptr)
	if hr := hresult(r0); hr < 0 {
		return hresultErr("AppendCategory", hr)
	}
	return nil
}

//nolint:gosec // G103: addresses of the receiver and of the collection, both live for the call.
func (l *destinationList) addUserTasks(items *objectCollection) error {
	r0 := callMethod(l.vtbl.AddUserTasks, uintptr(l.raw()), uintptr(unsafe.Pointer(items)))
	if hr := hresult(r0); hr < 0 {
		return hresultErr("AddUserTasks", hr)
	}
	return nil
}

func (l *destinationList) commit() error {
	return callPlain(l.vtbl.CommitList, l.raw(), "CommitList")
}

func (l *destinationList) abort() error {
	return callPlain(l.vtbl.AbortList, l.raw(), "AbortList")
}

func (l *destinationList) deleteList(appID string) error {
	return callWithString(l.vtbl.DeleteList, l.raw(), "DeleteList", appID)
}

func (l *destinationList) release() { release(l.raw()) }

//nolint:gosec // G103: pointer to the COM object itself.
func (a *objectArray) raw() unsafe.Pointer { return unsafe.Pointer(a) }

func (a *objectArray) release() { release(a.raw()) }

//nolint:gosec // G103: addresses of the receiver and of the counter, both live for the call.
func (a *objectArray) count() (uint32, error) {
	var n uint32
	r0 := callMethod(a.vtbl.GetCount, uintptr(a.raw()), uintptr(unsafe.Pointer(&n)))
	if hr := hresult(r0); hr < 0 {
		return 0, hresultErr("IObjectArray.GetCount", hr)
	}
	return n, nil
}

// shellLinkAt returns nil for a destination that is not a shell link (the
// list also hands back IShellItem destinations); those are not ours.
//
//nolint:gosec // G103: addresses of the receiver, the IID and the output pointer, all live for the call.
func (a *objectArray) shellLinkAt(i uint32) (*shellLink, error) {
	var obj unsafe.Pointer
	r0 := callMethod(a.vtbl.GetAt, uintptr(a.raw()), uintptr(i), uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&obj)))
	hr := hresult(r0)
	if hr == eNoInterface {
		return nil, nil
	}
	if hr < 0 {
		return nil, hresultErr("IObjectArray.GetAt", hr)
	}
	if obj == nil {
		return nil, errors.New("jumplist: IObjectArray.GetAt вернул нулевой указатель при успешном HRESULT")
	}
	return (*shellLink)(obj), nil
}

//nolint:gosec // G103: pointer to the COM object itself.
func (c *objectCollection) raw() unsafe.Pointer { return unsafe.Pointer(c) }

func (c *objectCollection) release() { release(c.raw()) }

//nolint:gosec // G103: addresses of the receiver and of the added object, both live for the call.
func (c *objectCollection) add(obj unsafe.Pointer) error {
	r0 := callMethod(c.vtbl.AddObject, uintptr(c.raw()), uintptr(obj))
	if hr := hresult(r0); hr < 0 {
		return hresultErr("IObjectCollection.AddObject", hr)
	}
	return nil
}

func newCollection() (*objectCollection, error) {
	obj, err := coCreate(&clsidEnumerableObjectCollection, &iidObjectCollection, "EnumerableObjectCollection")
	if err != nil {
		return nil, err
	}
	return (*objectCollection)(obj), nil
}

func newShellLink() (*shellLink, error) {
	obj, err := coCreate(&clsidShellLink, &iidShellLinkW, "IShellLinkW")
	if err != nil {
		return nil, err
	}
	return (*shellLink)(obj), nil
}

//nolint:gosec // G103: pointer to the COM object itself.
func (s *shellLink) raw() unsafe.Pointer { return unsafe.Pointer(s) }

func (s *shellLink) release() { release(s.raw()) }

func (s *shellLink) setPath(v string) error {
	return callWithString(s.vtbl.SetPath, s.raw(), "SetPath", v)
}

func (s *shellLink) setArguments(v string) error {
	return callWithString(s.vtbl.SetArguments, s.raw(), "SetArguments", v)
}

func (s *shellLink) setWorkingDirectory(v string) error {
	return callWithString(s.vtbl.SetWorkingDirectory, s.raw(), "SetWorkingDirectory", v)
}

//nolint:gosec // G103: addresses of the receiver and of the UTF-16 buffer, both live for the call.
func (s *shellLink) setIconLocation(path string) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("jumplist: SetIconLocation: кодирование %q: %w", path, err)
	}
	r0 := callMethod(s.vtbl.SetIconLocation, uintptr(s.raw()), uintptr(unsafe.Pointer(ptr)), 0)
	runtime.KeepAlive(ptr)
	if hr := hresult(r0); hr < 0 {
		return hresultErr("SetIconLocation", hr)
	}
	return nil
}

//nolint:gosec // G103: addresses of the receiver and of the buffer, both live for the call.
func (s *shellLink) arguments() (string, error) {
	var buf [argsBufLen]uint16
	r0 := callMethod(s.vtbl.GetArguments, uintptr(s.raw()), uintptr(unsafe.Pointer(&buf[0])), argsBufLen)
	if hr := hresult(r0); hr < 0 {
		return "", hresultErr("GetArguments", hr)
	}
	return windows.UTF16ToString(buf[:]), nil
}

func (s *shellLink) setTitle(title string) error {
	raw, _, err := queryInterface(s.raw(), &iidPropertyStore, "IPropertyStore")
	if err != nil {
		return err
	}
	store := (*propertyStore)(raw)
	defer release(raw)
	if err := store.setString(&pkeyTitle, title); err != nil {
		return err
	}
	return store.commit()
}

//nolint:gosec // G103: addresses of the receiver, the key and the variant, all live for the call.
func (p *propertyStore) setString(key *propertyKey, value string) error {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return fmt.Errorf("jumplist: SetValue: кодирование %q: %w", value, err)
	}
	pv := propVariant{vt: vtLPWSTR}
	pv.data[0] = unsafe.Pointer(ptr)
	r0 := callMethod(p.vtbl.SetValue, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&pv)))
	runtime.KeepAlive(ptr)
	if hr := hresult(r0); hr < 0 {
		return hresultErr("IPropertyStore.SetValue", hr)
	}
	return nil
}

//nolint:gosec // G103: address of the receiver, which is the COM object itself.
func (p *propertyStore) commit() error {
	return callPlain(p.vtbl.Commit, unsafe.Pointer(p), "IPropertyStore.Commit")
}

func removedArguments(removed *objectArray) (map[string]struct{}, error) {
	n, err := removed.count()
	if err != nil {
		return nil, err
	}
	args := make(map[string]struct{}, n)
	for i := range n {
		link, err := removed.shellLinkAt(i)
		if err != nil {
			return nil, err
		}
		if link == nil {
			continue
		}
		value, err := link.arguments()
		link.release()
		if err != nil {
			return nil, err
		}
		args[value] = struct{}{}
	}
	return args, nil
}

func buildCollection(cfg Config, entries []Entry) (*objectCollection, error) {
	collection, err := newCollection()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := addEntry(collection, cfg, entry); err != nil {
			collection.release()
			return nil, err
		}
	}
	return collection, nil
}

func addEntry(collection *objectCollection, cfg Config, entry Entry) error {
	link, err := newShellLink()
	if err != nil {
		return err
	}
	defer link.release()
	if err := link.setPath(cfg.Launcher); err != nil {
		return err
	}
	if err := link.setArguments(entry.Args); err != nil {
		return err
	}
	if err := link.setWorkingDirectory(filepath.Dir(cfg.Launcher)); err != nil {
		return err
	}
	if err := link.setIconLocation(entry.Icon); err != nil {
		return err
	}
	if err := link.setTitle(entry.Title); err != nil {
		return err
	}
	return collection.add(link.raw())
}

// The COM apartment belongs to the OS thread, so the whole call stays on one.
func Apply(cfg Config, games []Game) (err error) {
	if err := cfg.validate(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		return err
	}
	defer windows.CoUninitialize()

	list, err := createDestinationList()
	if err != nil {
		return err
	}
	defer list.release()
	if err := list.setAppID(cfg.AppID); err != nil {
		return err
	}
	removed, err := list.beginList()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if abortErr := list.abort(); abortErr != nil {
			err = errors.Join(err, abortErr)
		}
	}()

	gone, err := removedArguments(removed)
	removed.release()
	if err != nil {
		return err
	}
	recent, skipped := recentEntries(cfg, games, gone)
	for _, skip := range skipped {
		slog.Warn("jump list skips a game", "component", "jumplist", "error", skip)
	}
	if len(recent) > 0 {
		collection, err := buildCollection(cfg, recent)
		if err != nil {
			return err
		}
		defer collection.release()
		if err := list.appendCategory(cfg.Labels.Recent, collection); err != nil {
			return err
		}
	}
	tasks, err := buildCollection(cfg, taskEntries(cfg))
	if err != nil {
		return err
	}
	defer tasks.release()
	if err := list.addUserTasks(tasks); err != nil {
		return err
	}
	if err := list.commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func deleteList(appID string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		return err
	}
	defer windows.CoUninitialize()
	list, err := createDestinationList()
	if err != nil {
		return err
	}
	defer list.release()
	return list.deleteList(appID)
}
