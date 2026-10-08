//go:build windows

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRelaunchStartsTheLauncherWithoutArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	logPath := useApplyHelper(t, "ok")
	started := armApplyEvent(t)

	if err := relaunch(exe); err != nil {
		t.Fatalf("relaunch() error = %v", err)
	}
	if !awaitApplyEvent(t, started) {
		t.Fatal("the relaunched process never came up")
	}
	_, cmdLine := readApplyLog(t, logPath)
	args, err := windows.DecomposeCommandLine(cmdLine)
	if err != nil {
		t.Fatalf("DecomposeCommandLine(%q): %v", cmdLine, err)
	}
	if len(args) != 1 || args[0] != exe {
		t.Fatalf("relaunched command line = %q, want only the launcher itself: a stray argument would send it back into worker mode", args)
	}
}

func TestStartUpdateWorkerPassesTheSpecToTheCopy(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	specPath := filepath.Join(t.TempDir(), "update-spec.json")
	logPath := useApplyHelper(t, "ok")
	started := armApplyEvent(t)

	if err := startUpdateWorker(exe, specPath); err != nil {
		t.Fatalf("startUpdateWorker() error = %v", err)
	}
	if !awaitApplyEvent(t, started) {
		t.Fatal("the worker process never came up")
	}
	_, cmdLine := readApplyLog(t, logPath)
	args, err := windows.DecomposeCommandLine(cmdLine)
	if err != nil {
		t.Fatalf("DecomposeCommandLine(%q): %v", cmdLine, err)
	}
	if len(args) != 3 || args[0] != exe || args[1] != "--selfupdate-worker" || args[2] != specPath {
		t.Fatalf("worker command line = %q, want the copy, --selfupdate-worker and the spec path", args)
	}
}
