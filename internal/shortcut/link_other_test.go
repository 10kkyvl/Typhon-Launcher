//go:build !windows

package shortcut

import (
	"os"
	"testing"
)

func linkDir(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
