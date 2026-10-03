//go:build !windows

package install

import (
	"os"
	"testing"
)

func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore mode: %v", err)
		}
	})
	f, err := os.Open(path)
	if err == nil {
		if cerr := f.Close(); cerr != nil {
			t.Fatal(cerr)
		}
		t.Fatal("precondition: the file is still readable (running as root?)")
	}
}
