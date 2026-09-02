package library

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testExecutable returns a real, executable path present on the current OS
// (cmd.exe on Windows, /bin/sh elsewhere) together with the args that make
// it exit immediately, so session tests can launch a real short-lived
// process without depending on Windows-only paths.
func testExecutable(t *testing.T) (path string, exitArgs []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "cmd.exe"), []string{"/C", "exit"}
	}
	return "/bin/sh", []string{"-c", "exit 0"}
}

// testHoldArgs returns LaunchArgs that keep the process from testExecutable
// alive for roughly seconds before it exits on its own.
func testHoldArgs(seconds int) []string {
	if runtime.GOOS == "windows" {
		return []string{"/C", fmt.Sprintf("ping -n %d 127.0.0.1 >nul", seconds+1)}
	}
	return []string{"-c", fmt.Sprintf("sleep %d", seconds)}
}

// testPrintCwdArgs returns LaunchArgs that make the process from
// testExecutable print its working directory into cwd.txt.
func testPrintCwdArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"/C", "cd > cwd.txt"}
	}
	return []string{"-c", "pwd > cwd.txt"}
}

// testPlaceExecutable makes the real test executable runnable from dest, so
// a test can put it in an arbitrary directory. On Windows the bytes are
// copied, matching how a real installed game arrives on disk. Elsewhere a
// copy loses its code signature and gets SIGKILLed by the OS before it can
// run, so a symlink to the original is used instead.
func testPlaceExecutable(t *testing.T, dest string) string {
	t.Helper()
	src, _ := testExecutable(t)
	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // G703: dest is a path inside t.TempDir() chosen by the test, not user input
		if err := os.WriteFile(dest, data, 0o755); err != nil {
			t.Fatal(err)
		}
		return dest
	}
	if err := os.Symlink(src, dest); err != nil {
		t.Fatal(err)
	}
	return dest
}
