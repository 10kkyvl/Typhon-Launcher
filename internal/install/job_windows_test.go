//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func queryJobLimits(t *testing.T, job windows.Handle) windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION {
	t.Helper()
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	//nolint:gosec // G103: QueryInformationJobObject пишет в буфер вызывающего только через указатель
	ptr := uintptr(unsafe.Pointer(&info))
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, ptr, uint32(unsafe.Sizeof(info)), nil); err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	return info
}

func newTestJob(t *testing.T) windows.Handle {
	t.Helper()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatalf("CreateJobObject: %v", err)
	}
	t.Cleanup(func() {
		if err := windows.CloseHandle(job); err != nil {
			t.Logf("CloseHandle: %v", err)
		}
	})
	return job
}

func TestLimitJobSetsKillOnClose(t *testing.T) {
	job := newTestJob(t)
	if err := limitJob(job, true); err != nil {
		t.Fatalf("limitJob: %v", err)
	}
	info := queryJobLimits(t, job)
	flags := info.BasicLimitInformation.LimitFlags
	for name, flag := range map[string]uint32{
		"KILL_ON_JOB_CLOSE": windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		"PRIORITY_CLASS":    windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS,
		"SCHEDULING_CLASS":  windows.JOB_OBJECT_LIMIT_SCHEDULING_CLASS,
	} {
		if flags&flag == 0 {
			t.Errorf("job limit flags %#x lack %s", flags, name)
		}
	}
	if info.BasicLimitInformation.PriorityClass != windows.BELOW_NORMAL_PRIORITY_CLASS {
		t.Errorf("priority class = %#x, want BELOW_NORMAL", info.BasicLimitInformation.PriorityClass)
	}
}

func TestLimitJobForAnUninstallerKeepsItAlive(t *testing.T) {
	job := newTestJob(t)
	if err := limitJob(job, false); err != nil {
		t.Fatalf("limitJob: %v", err)
	}
	info := queryJobLimits(t, job)
	flags := info.BasicLimitInformation.LimitFlags
	if flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0 {
		t.Errorf("job limit flags %#x kill an uninstaller with its owner", flags)
	}
	if flags&windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS == 0 {
		t.Errorf("job limit flags %#x lost the priority limit", flags)
	}
}

func TestReleaseJobDropsOnlyKillOnClose(t *testing.T) {
	job := newTestJob(t)
	if err := limitJob(job, true); err != nil {
		t.Fatalf("limitJob: %v", err)
	}
	releaseJob(job, "installer.exe")
	info := queryJobLimits(t, job)
	flags := info.BasicLimitInformation.LimitFlags
	if flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0 {
		t.Errorf("job limit flags %#x still kill on close after release", flags)
	}
	if flags&windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS == 0 || info.BasicLimitInformation.PriorityClass != windows.BELOW_NORMAL_PRIORITY_CLASS {
		t.Errorf("release lost the priority limit: flags %#x, class %#x", flags, info.BasicLimitInformation.PriorityClass)
	}
}

func TestClosingGroupKillsBackgroundInstaller(t *testing.T) {
	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	//nolint:gosec // G204: cmd.exe — фиксированный системный бинарь тестового стенда, аргументы заданы тестом
	cmd := exec.Command(cmdExe, "/c", "ping", "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in installer: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("kill stand-in installer: %v", err)
		}
		<-done
	})

	group, err := groupProcess(cmd.Process.Pid, true, true)
	if err != nil {
		t.Fatalf("groupProcess: %v", err)
	}
	closeGroup(group, "installer.exe")

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("installer survived the close of its job object: kill-on-close is not in effect")
	}
}
