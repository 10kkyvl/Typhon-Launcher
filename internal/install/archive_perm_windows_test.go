//go:build windows

package install

import (
	"os"
	"os/exec"
	"testing"
)

// Go на Windows умеет лишь выставлять атрибут «только чтение», поэтому запрет
// чтения ставится правом доступа: файл остаётся на месте, а os.Open даёт
// ErrPermission.
func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	icacls, err := systemExecutable("icacls.exe")
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G204, invariant 33: icacls.exe is resolved through the system directory; path is a t.TempDir() file of the calling test
	out, err := exec.Command(icacls, path, "/deny", "*S-1-1-0:(R)").CombinedOutput()
	if err != nil {
		t.Fatalf("icacls deny: %v: %s", err, out)
	}
	t.Cleanup(func() {
		//nolint:gosec // G204, invariant 33: same system icacls.exe and t.TempDir() file as above
		if out, err := exec.Command(icacls, path, "/reset").CombinedOutput(); err != nil {
			t.Errorf("icacls reset: %v: %s", err, out)
		}
	})
	assertUnreadable(t, path)
}

func assertUnreadable(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err == nil {
		if cerr := f.Close(); cerr != nil {
			t.Fatal(cerr)
		}
		t.Fatal("precondition: the file is still readable")
	}
}
