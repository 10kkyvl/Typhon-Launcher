//go:build windows

package installguard

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

var fixtureReport = flag.String("guard.report", "", "fixture state file")

var externalGuard = flag.Bool("guard.external", false, "test a separate bridge monitoring this fixture")
var fixtureMusicClicks int

var defWindow = user32.NewProc("DefWindowProcW")
var fixtureProc = windows.NewCallback(func(h, m, w, l uintptr) uintptr {
	if m == 0x111 && w&0xffff == 1 && w>>16 == 0 {
		fixtureMusicClicks++
	}
	//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
	r, _, _ := defWindow.Call(h, m, w, l)
	return r
})

type fixtureClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type fixtureMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

func TestGuardControls(t *testing.T) {
	fixtureMusicClicks = 0
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wc := fixtureClass{Proc: fixtureProc, Name: windows.StringToUTF16Ptr("TWizardForm")}
	wc.Size = uint32(unsafe.Sizeof(wc))
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		t.Fatal(err)
	}
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(wc.Name)), 0)
	common := struct{ Size, Classes uint32 }{8, 0x20}
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	windows.NewLazySystemDLL("comctl32.dll").NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&common)))
	create := func(cls, label string, style, parent, id uintptr) uintptr {
		t.Helper()
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		h, _, e := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(cls))), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))), style, 20, 20+id*35, 300, 30, parent, id, 0, 0)
		if h == 0 {
			t.Fatalf("create %s: %v", cls, e)
		}
		return h
	}
	top := create("TWizardForm", "Typhon installer fixture", 0x10CF0000, 0, 0)
	//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
	defer user32.NewProc("DestroyWindow").Call(top)
	music := create("Button", "Play &music", 0x50000003, top, 1)
	soundtrack := create("Button", "Music files", 0x50000003, top, 2)
	website := create("Button", "Visit repacker website", 0x50000003, top, 5)
	uncheckedSite := create("Button", "Apply redirection to official site", 0x50000003, top, 6)
	create("msctls_progress32", "", 0x50000000, top, 3)
	next := create("Button", "Next", 0x40000000, top, 4)
	defer func() {
		if *fixtureReport != "" {
			state, _ := message(music, 0xF0, 0, 0)
			if err := os.WriteFile(*fixtureReport, []byte(fmt.Sprintf("failed=%v music=%d topStyle=%x nextStyle=%x", t.Failed(), state, style(top), style(next))), 0600); err != nil {
				t.Error(err)
			}
		}
	}()
	message(music, 0xF1, 1, 0)
	message(soundtrack, 0xF1, 1, 0)
	message(website, 0xF1, 1, 0)
	if !*externalGuard {
		stop := Start(context.Background(), os.Getpid(), true)
		defer stop()
	}
	peek := user32.NewProc("PeekMessageW")
	pump := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var msg fixtureMessage
			for {
				//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
				ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
				if ok == 0 {
					break
				}
				//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
				user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
				//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
				user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
			}
			if condition() {
				return
			}
			//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("guard did not update fixture controls before deadline")
	}
	pump(func() bool { v, _ := message(music, 0xF0, 0, 0); return v == 0 && style(top)&0x10000000 == 0 })
	pump(func() bool { v, _ := message(website, 0xF0, 0, 0); return v == 0 })
	if v, _ := message(uncheckedSite, 0xF0, 0, 0); v != 0 {
		t.Fatal("unchecked website option was enabled")
	}
	if fixtureMusicClicks != 1 {
		t.Fatalf("music OnClick calls = %d", fixtureMusicClicks)
	}
	if v, _ := message(soundtrack, 0xF0, 0, 0); v != 1 {
		t.Fatal("game music component was deselected")
	}
	// A newly enabled question must become reachable again after progress.
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	user32.NewProc("ShowWindow").Call(next, 4)
	pump(func() bool { return style(top)&0x10000000 != 0 })
	// Checked state is stable: do not toggle playback back on each polling pass.
	if v, _ := message(music, 0xF0, 0, 0); v != 0 {
		t.Fatal("music was enabled again")
	}
}

func TestGuardVerifier(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wc := fixtureClass{Proc: fixtureProc, Name: windows.StringToUTF16Ptr("QSFV_MAIN")}
	wc.Size = uint32(unsafe.Sizeof(wc))
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		t.Fatal(err)
	}
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(wc.Name)), 0)
	common := struct{ Size, Classes uint32 }{8, 1}
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	windows.NewLazySystemDLL("comctl32.dll").NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&common)))
	for _, summary := range []string{"All files OK", "1 file failed", ""} {
		t.Run(summary, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
			top, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wc.Name)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Finished"))), 0x10CF0000, 0, 0, 400, 200, 0, 0, 0, 0)
			if top == 0 {
				t.Fatal(err)
			}
			//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
			defer user32.NewProc("DestroyWindow").Call(top)
			//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
			list, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("SysListView32"))), 0, 0x50000001, 0, 0, 300, 100, top, 0, 0, 0)
			if list == 0 {
				t.Fatal(err)
			}
			item := struct {
				Mask             uint32
				Item, SubItem    int32
				State, StateMask uint32
				Text             *uint16
				MaxText, Image   int32
				Param            uintptr
				Indent           int32
			}{Mask: 1, Text: windows.StringToUTF16Ptr(summary)}
			if summary != "" {
				//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
				message(list, 0x104d, 0, uintptr(unsafe.Pointer(&item)))
			}

			count, _ := message(list, 0x1004, 0, 0)
			value, readOK := remoteListItemText(list, 0)
			if summary != "" && (count != 1 || !readOK || value != summary || text(top) != "Finished") {
				t.Fatalf("invalid verifier fixture: title=%q count=%d summary=%q read=%v", text(top), count, value, readOK)
			}
			quietVerifier(top)
			//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
			alive, _, _ := user32.NewProc("IsWindow").Call(top)
			// The production monitor retries timed-out window messages.
			for end := time.Now().Add(2 * time.Second); summary == "All files OK" && alive != 0 && time.Now().Before(end); {
				//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
				time.Sleep(20 * time.Millisecond)
				quietVerifier(top)
				//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
				alive, _, _ = user32.NewProc("IsWindow").Call(top)
			}
			if (alive == 0) != (summary == "All files OK") {
				t.Fatalf("summary=%q alive=%v", summary, alive)
			}
		})
	}
}
