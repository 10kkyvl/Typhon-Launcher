//go:build windows

package installguard

import (
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

var jobX86 = flag.String("job.x86", "", "32-bit fixture executable")

var jobRole = flag.String("job.role", "", "job test child role")
var jobReady = flag.String("job.ready", "", "job test ready file")

func TestJobFixtureProcess(t *testing.T) {
	if *jobRole == "" {
		return
	}

	if *jobRole == "allocation" || *jobRole == "allocation64" {
		for _, size := range []uintptr{0x80000000, 0x7fffffff, 0x7ffff001} {
			//nolint:errcheck // allocation probe asserts the returned address; failure is the expected outcome.
			large, _ := windows.VirtualAlloc(0, size, windows.MEM_RESERVE, windows.PAGE_READWRITE)
			if large != 0 {
				if err := windows.VirtualFree(large, 0, windows.MEM_RELEASE); err != nil {
					t.Error(err)
				}
			}
			if (large == 0) != (*jobRole == "allocation") {
				os.Exit(6)
			}
		}
		small, err := windows.VirtualAlloc(0, 16<<20, windows.MEM_RESERVE, windows.PAGE_READWRITE)
		if err != nil || small == 0 {
			os.Exit(7)
		}
		if err := windows.VirtualFree(small, 0, windows.MEM_RELEASE); err != nil {
			t.Error(err)
		}
		os.Exit(0)
	}
	if *jobRole == "parent" || *jobRole == "finite-parent" {
		exe, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		role := "child"
		if *jobRole == "finite-parent" {
			role = "finite-child"
		}
		//nolint:gosec // G204: launch this test executable from os.Executable with fixture-only arguments.
		child := exec.Command(exe, "-test.run=^TestJobFixtureProcess$", "-job.role="+role, "-job.ready="+*jobReady)
		if child.Start() != nil {
			os.Exit(3)
		}
		os.Exit(0) // The loader exits, leaving the actual installer alive.
	}
	if err := os.WriteFile(*jobReady, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(4)
	}
	if *jobRole == "finite-child" {
		//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
		time.Sleep(500 * time.Millisecond)
		if os.WriteFile(*jobReady+".complete", []byte("done"), 0600) != nil {
			os.Exit(5)
		}
		os.Exit(0)
	}
	for {
		//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
		time.Sleep(time.Second)
	}
}

func TestRunJobWaitsForNormalDescendantCompletion(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ready, cancel := filepath.Join(root, "ready"), filepath.Join(root, "cancel")
	timer := time.AfterFunc(5*time.Second, func() {
		if err := os.WriteFile(cancel, []byte("cancel"), 0600); err != nil {
			t.Error(err)
		}
	})
	defer timer.Stop()
	code, stopped, err := RunJob([]string{exe, "-test.run=^TestJobFixtureProcess$", "-job.role=finite-parent", "-job.ready=" + ready}, cancel, true, false)
	if code != 0 || !stopped || err != nil {
		t.Fatalf("completion: code=%d stopped=%t error=%v", code, stopped, err)
	}
	if _, err := os.Stat(cancel); err == nil {
		t.Fatal("normal completion required cancellation")
	}
	if data, err := os.ReadFile(ready + ".complete"); err != nil || string(data) != "done" {
		t.Fatalf("returned before final child write: %q %v", data, err)
	}
}

func TestRunJobCancelsDescendantAfterLoaderExit(t *testing.T) {
	for _, memoryGuard := range []bool{false, true} {
		t.Run(fmt.Sprint(memoryGuard), func(t *testing.T) { testJobCancellation(t, memoryGuard) })
	}
}

func testJobCancellation(t *testing.T, memoryGuard bool) {
	root := t.TempDir()
	cancel := filepath.Join(root, "cancel")
	ready := filepath.Join(root, "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		code    int
		stopped bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		code, stopped, err := RunJob([]string{exe, "-test.run=^TestJobFixtureProcess$", "-job.role=parent", "-job.ready=" + ready}, cancel, true, memoryGuard)
		done <- result{code, stopped, err}
	}()
	defer func() {
		if err := os.WriteFile(cancel, []byte("cancel"), 0600); err != nil {
			t.Error(err)
		}
	}()
	var pid int
	deadline := time.Now().Add(10 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if data, e := os.ReadFile(ready); e == nil {
			var parseErr error
			pid, parseErr = strconv.Atoi(string(data))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
		}
		select {
		case got := <-done:
			t.Fatalf("job returned before child cancellation: %+v", got)
		default:
		}
		//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child never became ready")
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(h)
	if err = os.WriteFile(cancel, []byte("cancel"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if !got.stopped || got.err != nil {
			t.Fatalf("cancellation=%+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("job cancellation blocked")
	}
	wait, err := windows.WaitForSingleObject(h, 0)
	if err != nil || wait != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived cancellation: %d %v", wait, err)
	}
}

func TestRunJobRejectsOversizedProbe(t *testing.T) {
	if *jobX86 == "" {
		t.Skip("set -job.x86 to 32-bit fixture")
	}
	native, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ exe, role string }{{*jobX86, "allocation"}, {native, "allocation64"}} {
		cancel := filepath.Join(t.TempDir(), "cancel")
		timer := time.AfterFunc(10*time.Second, func() {
			if err := os.WriteFile(cancel, []byte("cancel"), 0600); err != nil {
				t.Error(err)
			}
		})
		code, stopped, err := RunJob([]string{test.exe, "-test.run=^TestJobFixtureProcess$", "-job.role=" + test.role}, cancel, true, true)
		timer.Stop()
		if err != nil || !stopped || code != 0 {
			t.Fatalf("%s: code=%d stopped=%v err=%v", test.role, code, stopped, err)
		}
	}
}
