//go:build windows

package installguard

import (
	"context"
	"encoding/binary"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	enumWindows     = user32.NewProc("EnumWindows")
	enumChildren    = user32.NewProc("EnumChildWindows")
	windowPID       = user32.NewProc("GetWindowThreadProcessId")
	className       = user32.NewProc("GetClassNameW")
	windowLong      = user32.NewProc("GetWindowLongW")
	parentWindow    = user32.NewProc("GetParent")
	enabledWindow   = user32.NewProc("IsWindowEnabled")
	sendTimeout     = user32.NewProc("SendMessageTimeoutW")
	showWindow      = user32.NewProc("ShowWindowAsync")
	collectCallback = windows.NewCallback(func(hwnd, param uintptr) uintptr {
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		value, ok := callbackLists.Load(param)
		if !ok {
			return 0
		}
		list, ok := value.(*[]uintptr)
		if !ok {
			return 0
		}
		*list = append(*list, hwnd)
		return 1
	})
)

// Start follows descendants, including Inno's extracted setup.tmp. Held process
// handles prevent PID reuse from bringing unrelated applications into the tree.
// The returned stop function joins the monitor before installer cleanup proceeds.
func Start(ctx context.Context, pid int, hideProgress bool) func() {
	if pid <= 0 || uint64(pid) > 0xffffffff {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		//nolint:gosec // G115: Start rejects PIDs outside the positive uint32 range before starting the goroutine.
		watch(ctx, uint32(pid), hideProgress)
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }
}

func watch(ctx context.Context, root uint32, hide bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
		defer ole.CoUninitialize()
	}
	handles := map[uint32]windows.Handle{}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, root)
	if err != nil {
		return
	}
	handles[root] = h
	defer func() {
		for _, h := range handles {
			//nolint:errcheck // best-effort release of an owned native resource on exit.
			_ = windows.CloseHandle(h)
		}
	}()
	hidden := map[uintptr]bool{}
	traced := map[uintptr]string{}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		followChildren(handles)
		for _, hwnd := range listWindows(0) {
			if ctx.Err() != nil {
				return
			}
			var pid uint32
			//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
			windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if _, owned := handles[pid]; owned {
				if os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" {
					for _, control := range append([]uintptr{hwnd}, listWindows(hwnd)...) {
						signature := class(control) + "|" + text(control)
						if traced[control] == signature {
							continue
						}
						traced[control] = signature
						state, _ := message(control, 0xF0, 0, 0)
						slog.Info("installer control", "class", class(control), "title", text(control), "style", style(control), "check", state)
					}
				}
				quietWindow(ctx, hwnd, hide, hidden)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func followChildren(owned map[uint32]windows.Handle) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var entries []windows.ProcessEntry32
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		entries = append(entries, entry)
	}
	for changed := true; changed; {
		changed = false
		for _, p := range entries {
			if _, ok := owned[p.ProcessID]; ok {
				continue
			}
			if _, ok := owned[p.ParentProcessID]; !ok {
				continue
			}
			h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, p.ProcessID)
			if e == nil {
				owned[p.ProcessID] = h
				changed = true
			}
		}
	}
}

var callbackLists sync.Map
var callbackSequence atomic.Uint64

func collectList(list *[]uintptr) (uintptr, func()) {
	id := uintptr(callbackSequence.Add(1))
	callbackLists.Store(id, list)
	return id, func() { callbackLists.Delete(id) }
}

func listWindows(parent uintptr) []uintptr {
	var result []uintptr
	key, release := collectList(&result)
	defer release()
	if parent == 0 {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
		enumWindows.Call(collectCallback, key)
	} else {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
		enumChildren.Call(parent, collectCallback, key)
	}
	runtime.KeepAlive(&result)
	return result
}

func message(hwnd, msg, wp, lp uintptr) (uintptr, bool) {
	var result uintptr
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
	ok, _, _ := sendTimeout.Call(hwnd, msg, wp, lp, 0x2, 100, uintptr(unsafe.Pointer(&result))) // SMTO_ABORTIFHUNG
	return result, ok != 0
}

func text(hwnd uintptr) string {
	var buf [256]uint16
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	_, ok := message(hwnd, 0xD, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0]))) // WM_GETTEXT
	if !ok {
		return ""
	}
	return windows.UTF16ToString(buf[:])
}

func class(hwnd uintptr) string {
	var buf [128]uint16
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	className.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.ToLower(windows.UTF16ToString(buf[:]))
}

func style(hwnd uintptr) uintptr {
	//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
	v, _, _ := windowLong.Call(hwnd, ^uintptr(15)) // GWL_STYLE = -16
	return v
}

func visibleWithin(hwnd, top uintptr) bool {
	for hwnd != 0 && hwnd != top {
		if style(hwnd)&0x10000000 == 0 {
			return false
		} // WS_VISIBLE
		//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
		hwnd, _, _ = parentWindow.Call(hwnd)
	}
	return hwnd == top
}

func quietWindow(ctx context.Context, top uintptr, hide bool, hidden map[uintptr]bool) {
	progress, interactive := false, false
	topClass := class(top)
	if hide && topClass == "qsfv_main" {
		quietVerifier(top)
		return
	}

	wizard := topClass == "twizardform" || topClass == "tsetupform"
	for _, control := range listWindows(top) {
		if ctx.Err() != nil {
			return
		}
		cls := class(control)
		if cls == "tnewchecklistbox" {
			quietAccessibleOptions(control)
		}
		isButton := cls == "button" || cls == "tnewbutton" || cls == "tbutton" || cls == "tnewcheckbox" || cls == "tcheckbox"
		buttonStyle := style(control) & 0xF
		checkbox := isButton && (buttonStyle == 2 || buttonStyle == 3) // BS_CHECKBOX / BS_AUTOCHECKBOX
		label := ""
		if isButton {
			label = text(control)
		}
		if checkbox {
			wanted, ok := MusicState(label)
			if OptionalSiteAction(label) {
				wanted, ok = false, true
			}
			if ok {
				current, readOK := message(control, 0xF0, 0, 0) // BM_GETCHECK
				target := uintptr(0)
				if wanted {
					target = 1
				}
				if readOK && current <= 1 && current != target {
					// BM_CLICK runs the installer's OnClick handler; BM_SETCHECK alone
					// only changes the picture and can leave music playing.
					message(control, 0xF5, 0, 0)
				}
			}
		}
		if !wizard || !visibleWithin(control, top) {
			continue
		}
		if cls == "tnewprogressbar" || cls == "msctls_progress32" {
			progress = true
		}
		//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
		enabled, _, _ := enabledWindow.Call(control)
		if isButton && !checkbox && enabled != 0 {
			label = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(label, "&", "")))
			if label != "cancel" && label != "отмена" {
				interactive = true
			}
		}
	}
	// Hide only a recognised progress page. Custom questions and error dialogs
	// stay reachable, as do wizard pages requiring user input.
	if hide && wizard {
		if progress && !interactive {
			// STARTF_USESHOWWINDOW may already have hidden the initial progress
			// window. We still own restoring it if a later page asks a question.
			hidden[top] = true
			if style(top)&0x10000000 != 0 {
				//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
				showWindow.Call(top, 0)
			}
		} else if interactive && (hidden[top] || progress) {
			// ShowWindowAsync is queued and STARTF_USESHOWWINDOW may override the
			// first request. Keep restoring until visibility is actually observed.
			if style(top)&0x10000000 == 0 {
				//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
				showWindow.Call(top, 4)
			} else {
				delete(hidden, top)
			}
		}
	}
}

// QuickSFV is a blocking [Run] entry in these installers even with /VERYSILENT.
// Close only its completed, explicitly successful verification. Errors and
// unknown results stay visible; an idle process alone is not proof of success.
func quietVerifier(top uintptr) {
	if text(top) != "Finished" {
		if style(top)&0x10000000 != 0 {
			//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
			showWindow.Call(top, 0)
		}
		return
	}
	for _, control := range listWindows(top) {
		if class(control) != "syslistview32" {
			continue
		}
		count, ok := message(control, 0x1004, 0, 0) // LVM_GETITEMCOUNT
		if !ok || count == 0 {
			break
		}
		if result, ok := remoteListItemText(control, count-1); ok && result == "All files OK" {
			message(top, 0x10, 0, 0) // WM_CLOSE, after verification is finished
			return
		}
	}
	if style(top)&0x10000000 == 0 {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
		showWindow.Call(top, 4)
	}
}

func remoteListItemText(control, item uintptr) (string, bool) {
	var pid uint32
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	windowPID.Call(control, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_OPERATION|windows.PROCESS_VM_READ|windows.PROCESS_VM_WRITE, false, pid)
	if err != nil {
		return "", false
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(process)
	var wow64 bool
	if err := windows.IsWow64Process(process, &wow64); err != nil {
		return "", false
	}
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
	ptr, _, _ := kernel.NewProc("VirtualAllocEx").Call(uintptr(process), 0, 2048, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	if ptr == 0 {
		return "", false
	}
	free := true
	defer func() {
		if free {
			//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
			kernel.NewProc("VirtualFreeEx").Call(uintptr(process), ptr, 0, windows.MEM_RELEASE)
		}
	}()
	var descriptor [128]byte
	binary.LittleEndian.PutUint32(descriptor[:], 1) // LVIF_TEXT, subitem 0
	if wow64 || unsafe.Sizeof(uintptr(0)) == 4 {
		if uint64(ptr) > 0xffffffff-256 {
			return "", false
		}
		binary.LittleEndian.PutUint32(descriptor[20:], uint32(ptr+256))
		binary.LittleEndian.PutUint32(descriptor[24:], 1024)
	} else {
		binary.LittleEndian.PutUint64(descriptor[24:], uint64(ptr+256))
		binary.LittleEndian.PutUint32(descriptor[32:], 1024)
	}
	if err := windows.WriteProcessMemory(process, ptr, &descriptor[0], uintptr(len(descriptor)), nil); err != nil {
		return "", false
	}
	// LVM_GETITEMTEXTA uses a remote LVITEM, unlike marshalled WM_GETTEXT.
	length, ok := message(control, 0x102d, item, ptr)
	if !ok {
		free = false
		return "", false
	} // a delayed receiver may still use it
	if length == 0 || length >= 1024 {
		return "", false
	}
	var data [1024]byte
	if err := windows.ReadProcessMemory(process, ptr+256, &data[0], length, nil); err != nil {
		return "", false
	}
	return string(data[:length]), true
}
