package media

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// Interface IDs and vtable slots are taken from Windows SDK 10.0.26100.0:
// winrt/windows.media.control.h (MIDL_INTERFACE of each interface, methods in
// declaration order after the six IInspectable slots) and winrt/asyncinfo.h.
const (
	classSessionManager = "Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager"

	slotRequestAsync = 6

	slotGetCurrentSession = 6

	slotSourceAppUserModelID  = 6
	slotTryGetMediaProperties = 7
	slotGetPlaybackInfo       = 9
	slotTrySkipNext           = 16
	slotTrySkipPrevious       = 17
	slotTryTogglePlayPause    = 20

	slotPropsTitle      = 6
	slotPropsArtist     = 9
	slotPropsAlbumTitle = 10

	slotInfoControls       = 6
	slotInfoPlaybackStatus = 7

	slotControlsNext      = 12
	slotControlsPrevious  = 13
	slotControlsPlayPause = 16

	slotAsyncGetResults = 8

	slotAsyncStatus    = 7
	slotAsyncErrorCode = 8
	slotAsyncCancel    = 9

	slotQueryInterface = 0
	slotRelease        = 2

	playbackStatusPlaying = 4

	asyncCompleted = 1
	asyncCanceled  = 2
	asyncError     = 3

	asyncPoll = 5 * time.Millisecond

	roInitMultiThreaded = 1
	sFalse              = 1
)

var (
	iidSessionManagerStatics = ole.NewGUID("2050c4ee-11a0-57de-aed7-c97c70338245")
	iidAsyncInfo             = ole.NewGUID("00000036-0000-0000-c000-000000000046")

	procRoUninitialize = windows.NewLazySystemDLL("combase.dll").NewProc("RoUninitialize")
)

type hresultError struct {
	op string
	hr uint32
}

func (e *hresultError) Error() string {
	return fmt.Sprintf("%s: HRESULT 0x%08X", e.op, e.hr)
}

func failed(hr uintptr) bool { return hr&0x80000000 != 0 }

type object struct{ p unsafe.Pointer }

//nolint:gosec // G103: a COM object pointer addresses its vtable pointer, and the slot is read from the vtable that object publishes; both live in native memory owned by the object.
func (o object) slot(idx uintptr) uintptr {
	vt := *(*unsafe.Pointer)(o.p)
	return *(*uintptr)(unsafe.Add(vt, idx*unsafe.Sizeof(uintptr(0))))
}

// uintptrescapes keeps the out variables that callers pass as
// uintptr(unsafe.Pointer(&v)) on the heap, so a stack move cannot invalidate
// the address between the conversion and the call.
//
//go:uintptrescapes
//nolint:errcheck,gosec // errcheck: SyscallN reports failure through the HRESULT, which is returned, and its Errno is unrelated to COM. G103: the callee writes through the out pointers it is handed.
func (o object) call(op string, idx uintptr, args ...uintptr) error {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(o.p))
	all = append(all, args...)
	hr, _, _ := syscall.SyscallN(o.slot(idx), all...)
	if failed(hr) {
		return &hresultError{op: op, hr: uint32(hr)}
	}
	return nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o object) child(op string, idx uintptr) (object, error) {
	var out unsafe.Pointer
	if err := o.call(op, idx, uintptr(unsafe.Pointer(&out))); err != nil {
		return object{}, err
	}
	return object{p: out}, nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o object) queryInterface(iid *ole.GUID) (object, error) {
	var out unsafe.Pointer
	if err := o.call("QueryInterface", slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); err != nil {
		return object{}, err
	}
	return object{p: out}, nil
}

//nolint:errcheck // Release returns the remaining reference count, not an error.
func (o object) release() {
	if o.p == nil {
		return
	}
	syscall.SyscallN(o.slot(slotRelease), uintptr(o.p))
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o object) str(op string, idx uintptr) (string, error) {
	var h ole.HString
	if err := o.call(op, idx, uintptr(unsafe.Pointer(&h))); err != nil {
		return "", err
	}
	if h == 0 {
		return "", nil
	}
	s := h.String()
	if err := ole.DeleteHString(h); err != nil {
		return "", fmt.Errorf("%s: free string: %w", op, err)
	}
	return s, nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o object) flag(op string, idx uintptr) (bool, error) {
	var b uint8
	if err := o.call(op, idx, uintptr(unsafe.Pointer(&b))); err != nil {
		return false, err
	}
	return b != 0, nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o object) int32Value(op string, idx uintptr) (int32, error) {
	var v int32
	if err := o.call(op, idx, uintptr(unsafe.Pointer(&v))); err != nil {
		return 0, err
	}
	return v, nil
}

func roInitialize() error {
	err := ole.RoInitialize(roInitMultiThreaded)
	if err == nil {
		return nil
	}
	var oe *ole.OleError
	if errors.As(err, &oe) && oe.Code() == sFalse {
		return nil
	}
	return fmt.Errorf("RoInitialize: %w", err)
}

//nolint:errcheck,gosec // RoUninitialize returns nothing and sets no last error.
func roUninitialize() {
	procRoUninitialize.Call()
}

func activationFactory() (object, error) {
	ins, err := ole.RoGetActivationFactory(classSessionManager, iidSessionManagerStatics)
	if err != nil {
		return object{}, fmt.Errorf("RoGetActivationFactory: %w", err)
	}
	return object{p: unsafe.Pointer(ins)}, nil //nolint:gosec // G103: the factory is a native COM pointer returned by RoGetActivationFactory.
}

type asyncStatus interface {
	status() (int32, error)
	errorCode() (uint32, error)
	cancel()
}

type asyncInfo struct{ obj object }

func (a asyncInfo) status() (int32, error) {
	return a.obj.int32Value("IAsyncInfo.Status", slotAsyncStatus)
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (a asyncInfo) errorCode() (uint32, error) {
	var hr uint32
	if err := a.obj.call("IAsyncInfo.ErrorCode", slotAsyncErrorCode, uintptr(unsafe.Pointer(&hr))); err != nil {
		return 0, err
	}
	return hr, nil
}

func (a asyncInfo) cancel() {
	_ = a.obj.call("IAsyncInfo.Cancel", slotAsyncCancel) //nolint:errcheck // best-effort cancel of an operation nobody waits for any more; the caller returns ctx.Err() either way.
}

func waitAsync(ctx context.Context, a asyncStatus, interval time.Duration) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		st, err := a.status()
		if err != nil {
			return err
		}
		switch st {
		case asyncCompleted:
			return nil
		case asyncCanceled:
			return errors.New("async operation was canceled")
		case asyncError:
			hr, err := a.errorCode()
			if err != nil {
				return fmt.Errorf("async operation failed: %w", err)
			}
			return &hresultError{op: "async operation", hr: hr}
		}
		select {
		case <-ctx.Done():
			a.cancel()
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func await(ctx context.Context, op object) error {
	if op.p == nil {
		return errors.New("no async operation returned")
	}
	info, err := op.queryInterface(iidAsyncInfo)
	if err != nil {
		return err
	}
	defer info.release()
	return waitAsync(ctx, asyncInfo{obj: info}, asyncPoll)
}

func awaitResult(ctx context.Context, op object) (object, error) {
	defer op.release()
	if err := await(ctx, op); err != nil {
		return object{}, err
	}
	return op.child("IAsyncOperation.GetResults", slotAsyncGetResults)
}

func awaitFlag(ctx context.Context, op object) (bool, error) {
	defer op.release()
	if err := await(ctx, op); err != nil {
		return false, err
	}
	return op.flag("IAsyncOperation.GetResults", slotAsyncGetResults)
}
