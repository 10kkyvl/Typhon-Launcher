//go:build !windows

package protocolreg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunsFromFollowsASymlink(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Typhon")
	touch(t, filepath.Join(install, "typhon.exe"))
	link := filepath.Join(root, "link")
	if err := os.Symlink(install, link); err != nil {
		t.Fatal(err)
	}
	got, err := RunsFrom(filepath.Join(link, "typhon.exe"), install)
	if err != nil || !got {
		t.Fatalf("RunsFrom = %v, %v", got, err)
	}
}
