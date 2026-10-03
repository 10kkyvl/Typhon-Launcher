//go:build windows

package installguard

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const verifierHelperEnv = "TYPHON_VERIFIER_HELPER"

func TestVerifierHelperProcess(t *testing.T) {
	if os.Getenv(verifierHelperEnv) != "1" {
		return
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
}

func verifierWindow(t *testing.T, title string) uintptr {
	t.Helper()
	name := windows.StringToUTF16Ptr("QSFV_MAIN")
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	top, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(title))), 0x10CF0000, 0, 0, 400, 200, 0, 0, 0, 0)
	if top == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:errcheck,gosec // the window may already be destroyed by the code under test. G104: native result is not authoritative, cleanup is best effort.
		user32.NewProc("DestroyWindow").Call(top)
	})
	return top
}

func registerVerifierClass(t *testing.T) {
	t.Helper()
	wc := fixtureClass{Proc: fixtureProc, Name: windows.StringToUTF16Ptr("QSFV_MAIN")}
	wc.Size = uint32(unsafe.Sizeof(wc))
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G103: synchronous Win32/COM ABI call with live typed buffers.
		user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(wc.Name)), 0)
	})
}

func windowAlive(top uintptr) bool {
	//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
	alive, _, _ := user32.NewProc("IsWindow").Call(top)
	return alive != 0
}

func TestQuietWindowVerifierPolicy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	registerVerifierClass(t)
	pid := windows.GetCurrentProcessId()
	for _, tc := range []struct {
		name      string
		opts      Options
		wantAlive bool
	}{
		{"verification allowed keeps unfinished verifier", Options{HideProgress: true, VerifyRepack: true}, true},
		{"verification skipped closes verifier at once", Options{HideProgress: true}, false},
		{"no progress hiding leaves verifier alone", Options{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			top := verifierWindow(t, "Verifying")
			quietWindow(context.Background(), top, pid, tc.opts, map[uintptr]bool{}, newVerifierSkip())
			if got := windowAlive(top); got != tc.wantAlive {
				t.Fatalf("window alive = %v, want %v", got, tc.wantAlive)
			}
		})
	}
}

func TestVerifierSkipClosesOncePerProcess(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	registerVerifierClass(t)
	skip := newVerifierSkip()
	pid := windows.GetCurrentProcessId()
	first := verifierWindow(t, "Verifying")
	second := verifierWindow(t, "Verifying")
	skip.window(first, pid)
	if windowAlive(first) {
		t.Fatal("first verifier window survived WM_CLOSE")
	}
	skip.window(second, pid)
	if !windowAlive(second) {
		t.Fatal("WM_CLOSE was sent again to a process already asked to close")
	}
	if _, ok := skip.closed[pid]; !ok {
		t.Fatal("closed process is not tracked for the kill fallback")
	}
}

func verifierChild(t *testing.T) (uint32, windows.Handle) {
	t.Helper()
	//nolint:gosec // G204: the test binary re-executes itself as a stand-in process; no external input.
	cmd := exec.Command(os.Args[0], "-test.run=^TestVerifierHelperProcess$")
	cmd.Env = append(os.Environ(), verifierHelperEnv+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil {
			t.Logf("close helper stdin: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Logf("helper exit: %v", err)
		}
	})
	//nolint:gosec // G115: pid, выданный ядром, в uint32 помещается всегда
	pid := uint32(cmd.Process.Pid)
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := windows.CloseHandle(h); err != nil {
			t.Logf("close helper handle: %v", err)
		}
	})
	return pid, h
}

func TestVerifierSweepTerminatesStuckProcess(t *testing.T) {
	pid, h := verifierChild(t)
	handles := map[uint32]windows.Handle{pid: h}
	skip := newVerifierSkip()
	skip.closed[pid] = 0

	for range verifierKillAfter - 1 {
		skip.sweep(handles)
	}
	if exited, err := processExited(h); err != nil || exited {
		t.Fatalf("verifier ended inside the grace period: exited=%v err=%v", exited, err)
	}
	if _, ok := skip.closed[pid]; !ok {
		t.Fatal("verifier forgotten before the grace period ended")
	}

	skip.sweep(handles)
	event, err := windows.WaitForSingleObject(h, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if event != windows.WAIT_OBJECT_0 {
		t.Fatalf("verifier still running after the grace period, wait=%d", event)
	}
	if _, ok := skip.closed[pid]; ok {
		t.Fatal("terminated verifier is still tracked")
	}
}

func TestVerifierSweepIgnoresForeignProcess(t *testing.T) {
	pid, h := verifierChild(t)
	skip := newVerifierSkip()
	skip.closed[pid] = verifierKillAfter * 2
	skip.sweep(map[uint32]windows.Handle{})
	if exited, err := processExited(h); err != nil || exited {
		t.Fatalf("process outside the installer tree was terminated: exited=%v err=%v", exited, err)
	}
	if _, ok := skip.closed[pid]; ok {
		t.Fatal("process outside the installer tree is still tracked")
	}
}
