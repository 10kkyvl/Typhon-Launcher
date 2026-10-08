//go:build windows

package installguard

import (
	"errors"
	"fmt"
	"log/slog"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
)

// Interface IDs and vtable slots are taken from Windows SDK 10.0.26100.0:
// um/mmdeviceapi.h, um/audiopolicy.h and um/audioclient.h, methods counted in
// declaration order after the three IUnknown slots.
const (
	slotRelease = 2

	slotEnumAudioEndpoints = 3

	slotCollectionCount = 3
	slotCollectionItem  = 4

	slotDeviceActivate = 3

	slotSessionEnumerator = 5

	slotSessionCount = 3
	slotSessionItem  = 4

	slotSessionProcessID = 14

	slotSetMute = 5
	slotGetMute = 6

	eRender            = 0
	deviceStateActive  = 1
	clsctxInprocServer = 1
)

var (
	clsidDeviceEnumerator = ole.NewGUID("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidDeviceEnumerator   = ole.NewGUID("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidSessionManager2    = ole.NewGUID("77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F")
	iidSessionControl2    = ole.NewGUID("BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D")
	iidSimpleVolume       = ole.NewGUID("87CE5498-68D6-44E5-9215-6DA47EF883D8")
)

type hresultError struct {
	op string
	hr uint32
}

func (e *hresultError) Error() string {
	return fmt.Sprintf("%s: HRESULT 0x%08X", e.op, e.hr)
}

type comObject struct{ p unsafe.Pointer }

//nolint:gosec // G103: a COM object pointer addresses its vtable pointer, and the slot is read from the vtable that object publishes; both live in native memory owned by the object.
func (o comObject) slot(idx uintptr) uintptr {
	vt := *(*unsafe.Pointer)(o.p)
	return *(*uintptr)(unsafe.Add(vt, idx*unsafe.Sizeof(uintptr(0))))
}

// uintptrescapes keeps the out variables that callers pass as
// uintptr(unsafe.Pointer(&v)) on the heap, so a stack move cannot invalidate
// the address between the conversion and the call.
//
//go:uintptrescapes
//nolint:errcheck,gosec // errcheck: SyscallN reports failure through the HRESULT, which is returned, and its Errno is unrelated to COM. G103: the callee writes through the out pointers it is handed.
func (o comObject) call(op string, idx uintptr, args ...uintptr) (uintptr, error) {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(o.p))
	all = append(all, args...)
	hr, _, _ := syscall.SyscallN(o.slot(idx), all...)
	if hr&0x80000000 != 0 {
		return hr, &hresultError{op: op, hr: uint32(hr)}
	}
	return hr, nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o comObject) child(op string, idx uintptr, args ...uintptr) (comObject, error) {
	var out unsafe.Pointer
	if _, err := o.call(op, idx, append(args, uintptr(unsafe.Pointer(&out)))...); err != nil {
		return comObject{}, err
	}
	if out == nil {
		return comObject{}, fmt.Errorf("%s: no object returned", op)
	}
	return comObject{p: out}, nil
}

//nolint:gosec // G103: the IID is a package-level GUID that outlives the call.
func (o comObject) query(iid *ole.GUID) (comObject, error) {
	return o.child("QueryInterface", 0, uintptr(unsafe.Pointer(iid)))
}

func (o comObject) release() {
	if o.p != nil {
		o.call("Release", slotRelease) //nolint:errcheck,gosec // errcheck, G104: Release returns the new reference count, not a status.
	}
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (o comObject) count(op string, idx uintptr) (uint32, error) {
	var n uint32
	if _, err := o.call(op, idx, uintptr(unsafe.Pointer(&n))); err != nil {
		return 0, err
	}
	return n, nil
}

// audioSession is the part of a render session the guard needs; it lets the
// muting rule be tested without a sound card.
type audioSession interface {
	processID() (uint32, error)
	muted() (bool, error)
	mute() error
}

// muteInstallerAudio silences every session that belongs to the installer
// process tree. Repacks play music through their own players (FitGirl draws
// the toggle as an image, botva2 keeps its state out of reach), so muting the
// sessions is the one switch that works for all of them.
func muteInstallerAudio(sessions []audioSession, owned func(uint32) bool) (int, error) {
	silenced := 0
	var failures []error
	for _, s := range sessions {
		pid, err := s.processID()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if pid == 0 || !owned(pid) {
			continue
		}
		already, err := s.muted()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if already {
			continue
		}
		if err := s.mute(); err != nil {
			failures = append(failures, err)
			continue
		}
		silenced++
	}
	return silenced, errors.Join(failures...)
}

type coreSession struct {
	control comObject
	volume  comObject
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (s coreSession) processID() (uint32, error) {
	var pid uint32
	if _, err := s.control.call("IAudioSessionControl2.GetProcessId", slotSessionProcessID, uintptr(unsafe.Pointer(&pid))); err != nil {
		return 0, err
	}
	return pid, nil
}

//nolint:gosec // G103: the out pointer is the address of a local that outlives the call, kept on the heap by uintptrescapes on call.
func (s coreSession) muted() (bool, error) {
	var state int32
	if _, err := s.volume.call("ISimpleAudioVolume.GetMute", slotGetMute, uintptr(unsafe.Pointer(&state))); err != nil {
		return false, err
	}
	return state != 0, nil
}

func (s coreSession) mute() error {
	_, err := s.volume.call("ISimpleAudioVolume.SetMute", slotSetMute, 1, 0)
	return err
}

func (s coreSession) release() {
	s.volume.release()
	s.control.release()
}

// renderSessions lists the sessions of every active playback device: the
// installer may play on a device other than the default one. The caller must
// have initialised COM on this thread and releases what it gets back.
//
//nolint:gosec // G103: IIDs and out pointers are passed to COM for the duration of the synchronous calls.
func renderSessions() ([]coreSession, error) {
	unknown, err := ole.CreateInstance(clsidDeviceEnumerator, iidDeviceEnumerator)
	if err != nil {
		return nil, fmt.Errorf("create MMDeviceEnumerator: %w", err)
	}
	enumerator := comObject{p: unsafe.Pointer(unknown)}
	defer enumerator.release()

	devices, err := enumerator.child("IMMDeviceEnumerator.EnumAudioEndpoints", slotEnumAudioEndpoints, eRender, deviceStateActive)
	if err != nil {
		return nil, err
	}
	defer devices.release()
	n, err := devices.count("IMMDeviceCollection.GetCount", slotCollectionCount)
	if err != nil {
		return nil, err
	}

	var out []coreSession
	var failures []error
	for i := uint32(0); i < n; i++ {
		found, err := deviceSessions(devices, i)
		out = append(out, found...)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return out, errors.Join(failures...)
}

//nolint:gosec // G103: IIDs and out pointers are passed to COM for the duration of the synchronous calls.
func deviceSessions(devices comObject, index uint32) ([]coreSession, error) {
	device, err := devices.child("IMMDeviceCollection.Item", slotCollectionItem, uintptr(index))
	if err != nil {
		return nil, err
	}
	defer device.release()
	manager, err := device.child("IMMDevice.Activate", slotDeviceActivate, uintptr(unsafe.Pointer(iidSessionManager2)), clsctxInprocServer, 0)
	if err != nil {
		return nil, err
	}
	defer manager.release()
	list, err := manager.child("IAudioSessionManager2.GetSessionEnumerator", slotSessionEnumerator)
	if err != nil {
		return nil, err
	}
	defer list.release()
	n, err := list.count("IAudioSessionEnumerator.GetCount", slotSessionCount)
	if err != nil {
		return nil, err
	}

	var out []coreSession
	var failures []error
	for i := uint32(0); i < n; i++ {
		base, err := list.child("IAudioSessionEnumerator.GetSession", slotSessionItem, uintptr(i))
		if err != nil {
			failures = append(failures, err)
			continue
		}
		control, err := base.query(iidSessionControl2)
		if err != nil {
			base.release()
			failures = append(failures, err)
			continue
		}
		volume, err := base.query(iidSimpleVolume)
		base.release()
		if err != nil {
			control.release()
			failures = append(failures, err)
			continue
		}
		out = append(out, coreSession{control: control, volume: volume})
	}
	return out, errors.Join(failures...)
}

func muteOwnedAudio(owned func(uint32) bool) (int, error) {
	sessions, listErr := renderSessions()
	defer func() {
		for _, s := range sessions {
			s.release()
		}
	}()
	view := make([]audioSession, len(sessions))
	for i, s := range sessions {
		view[i] = s
	}
	silenced, err := muteInstallerAudio(view, owned)
	return silenced, errors.Join(listErr, err)
}

// audioReport keeps the 200 ms guard loop from repeating the same log line.
type audioReport struct {
	silenced int
	lastErr  string
}

func (r *audioReport) note(silenced int, err error) {
	if silenced > 0 {
		r.silenced += silenced
		slog.Info("installer audio muted", "sessions", silenced, "total", r.silenced)
	}
	if err == nil {
		r.lastErr = ""
		return
	}
	if text := err.Error(); text != r.lastErr {
		r.lastErr = text
		slog.Warn("mute installer audio", "error", err)
	}
}
