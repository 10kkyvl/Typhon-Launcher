//go:build windows

package installguard

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const (
	verifierHelperEnv = "TYPHON_VERIFIER_HELPER"
	verifierClass     = "QSFV_MAIN"
	stubbornClass     = "TYPHON_STUBBORN_VERIFIER"
	wmClose           = 0x10
	wmQuit            = 0x12
)

var onStubbornClose func()

var stubbornProc = windows.NewCallback(func(h, m, w, l uintptr) uintptr {
	if m == wmClose {
		onStubbornClose()
		return 0
	}
	return win32(defWindow, h, m, w, l)
})

func TestVerifierHelperProcess(t *testing.T) {
	switch os.Getenv(verifierHelperEnv) {
	case "1":
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
	case "window":
		runStubbornVerifier(t)
	}
}

func say(line string) {
	fmt.Fprintln(os.Stderr, line)
}

func runStubbornVerifier(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	registerWindowClass(t, verifierClass, stubbornProc)
	onStubbornClose = func() { say("wm_close") }
	createWindow(t, verifierClass, "Verifying")
	thread := windows.GetCurrentThreadId()
	go func() {
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			say("helper stdin: " + err.Error())
		}
		if win32(postThreadMessage, uintptr(thread), wmQuit, 0, 0) == 0 {
			say("helper could not post WM_QUIT")
		}
	}()
	say("ready")
	for {
		var msg fixtureMessage
		if got := msgCall(getMessage, &msg) & 0xFFFFFFFF; got == 0 || got == 0xFFFFFFFF {
			return
		}
		msgCall(translateMessage, &msg)
		msgCall(dispatchMessage, &msg)
	}
}

func verifierWindow(t *testing.T, title string) uintptr {
	t.Helper()
	return createWindow(t, verifierClass, title)
}

func registerVerifierClass(t *testing.T) {
	t.Helper()
	registerWindowClass(t, verifierClass, fixtureProc)
}

func lockThread(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
}

func TestQuietWindowVerifierPolicy(t *testing.T) {
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
			lockThread(t)
			top := verifierWindow(t, "Verifying")
			quietWindow(context.Background(), top, pid, tc.opts, map[uintptr]bool{}, newVerifierSkip())
			if got := windowAlive(top); got != tc.wantAlive {
				t.Fatalf("window alive = %v, want %v", got, tc.wantAlive)
			}
		})
	}
}

func TestVerifierSkipClosesOncePerProcess(t *testing.T) {
	registerVerifierClass(t)
	lockThread(t)
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
	cmd := reexec("1")
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
	skip.closed[pid] = &verifierProc{}

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
	skip.closed[pid] = &verifierProc{passes: verifierKillAfter * 2}
	skip.sweep(map[uint32]windows.Handle{})
	if exited, err := processExited(h); err != nil || exited {
		t.Fatalf("process outside the installer tree was terminated: exited=%v err=%v", exited, err)
	}
	if _, ok := skip.closed[pid]; ok {
		t.Fatal("process outside the installer tree is still tracked")
	}
}

func TestVerifierSweepGivesUpWhenEndingFails(t *testing.T) {
	registerWindowClass(t, stubbornClass, stubbornProc)
	pid := windows.GetCurrentProcessId()
	for _, tc := range []struct {
		name           string
		exited         func(windows.Handle) (bool, error)
		terminateErr   error
		wantTerminates int
		wantStep       string
	}{
		{"terminate refused", func(windows.Handle) (bool, error) { return false, nil }, errors.New("access denied"), 1, "step=terminate"},
		{"liveness check refused", func(windows.Handle) (bool, error) { return false, errors.New("invalid handle") }, nil, 0, "step=wait"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lockThread(t)
			closes := 0
			onStubbornClose = func() { closes++ }
			t.Cleanup(func() { onStubbornClose = nil })
			var logged bytes.Buffer
			terminates := 0
			skip := newVerifierSkip()
			skip.log = slog.New(slog.NewTextHandler(&logged, nil))
			skip.exited = tc.exited
			skip.terminate = func(uint32) error {
				terminates++
				return tc.terminateErr
			}
			var shown []uintptr
			skip.show = func(top, command uintptr) {
				shown = append(shown, command)
				showSync(top, command)
			}
			handles := map[uint32]windows.Handle{pid: 0}
			top := createWindow(t, stubbornClass, "Verifying")

			// Like the guard, repeat a request until the window state is seen to change.
			for range 100 {
				skip.window(top, pid)
				if style(top)&wsVisible == 0 {
					break
				}
			}
			if style(top)&wsVisible != 0 {
				t.Fatal("verifier window was not hidden")
			}
			for range verifierKillAfter + 5 {
				skip.sweep(handles)
				skip.window(top, pid)
			}
			for range 100 {
				if style(top)&wsVisible != 0 {
					break
				}
				skip.window(top, pid)
			}

			if style(top)&wsVisible == 0 {
				t.Fatal("verifier window stayed hidden after the guard gave up")
			}
			if len(shown) < 2 || shown[0] != swHide || shown[len(shown)-1] != swShowNoActivate || !slices.IsSorted(shown) {
				t.Fatalf("window show commands = %v, want hides followed by shows and no hide after the guard gave up", shown)
			}
			if closes != 1 {
				t.Fatalf("WM_CLOSE sent %d times, want 1", closes)
			}
			if terminates != tc.wantTerminates {
				t.Fatalf("terminate called %d times, want %d", terminates, tc.wantTerminates)
			}
			if n := strings.Count(logged.String(), "msg=\"repack verifier left running\""); n != 1 {
				t.Fatalf("failure logged %d times, want 1:\n%s", n, logged.String())
			}
			if !strings.Contains(logged.String(), tc.wantStep) {
				t.Fatalf("log does not name the failed step %q:\n%s", tc.wantStep, logged.String())
			}
			if n := strings.Count(logged.String(), "msg=\"repack verification skipped\""); n != 1 {
				t.Fatalf("skip logged %d times, want 1:\n%s", n, logged.String())
			}
			if tracked, ok := skip.closed[pid]; !ok || !tracked.gaveUp {
				t.Fatal("gave-up verifier is not remembered")
			}
		})
	}
}

func TestVerifierSweepForgetsGaveUpProcessAfterExit(t *testing.T) {
	pid := uint32(4242)
	exited := false
	skip := newVerifierSkip()
	skip.log = slog.New(slog.DiscardHandler)
	skip.terminate = func(uint32) error { return errors.New("access denied") }
	skip.exited = func(windows.Handle) (bool, error) { return exited, nil }
	skip.closed[pid] = &verifierProc{passes: verifierKillAfter}
	handles := map[uint32]windows.Handle{pid: 0}

	skip.sweep(handles)
	if tracked, ok := skip.closed[pid]; !ok || !tracked.gaveUp {
		t.Fatal("verifier was not given up on")
	}
	exited = true
	skip.sweep(handles)
	if _, ok := skip.closed[pid]; ok {
		t.Fatal("exited verifier is still tracked")
	}
}

func TestTerminateVerifierReportsFailure(t *testing.T) {
	if err := terminateVerifier(0xFFFFFFFC); err == nil {
		t.Fatal("terminating a process that does not exist reported success")
	}
}

func TestGuardEndsVerifierThatIgnoresClose(t *testing.T) {
	previous := guardInterval
	guardInterval = 20 * time.Millisecond
	t.Cleanup(func() { guardInterval = previous })

	cmd := reexec("window")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		scan := bufio.NewScanner(stderr)
		for scan.Scan() {
			lines <- scan.Text()
		}
	}()
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil {
			t.Logf("close helper stdin: %v", err)
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Logf("stop helper: %v", err)
		}
	})
	deadline := time.After(30 * time.Second)
	select {
	case line := <-lines:
		if line != "ready" {
			t.Fatalf("helper said %q before the window existed", line)
		}
	case <-deadline:
		t.Fatal("helper window did not appear")
	}

	stop := Start(context.Background(), cmd.Process.Pid, Options{HideProgress: true})
	defer stop()
	var seen []string
	for open := true; open; {
		select {
		case line, ok := <-lines:
			if !ok {
				open = false
				break
			}
			seen = append(seen, line)
		case <-deadline:
			t.Fatalf("verifier was not terminated; helper output: %q", seen)
		}
	}
	if len(seen) != 1 || seen[0] != "wm_close" {
		t.Fatalf("helper output = %q, want exactly one WM_CLOSE before the kill", seen)
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("helper did not end by TerminateProcess: %v", err)
	}
}
