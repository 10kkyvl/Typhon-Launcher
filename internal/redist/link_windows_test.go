//go:build windows

package redist

import (
	"os/exec"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func TestInstallRefusesThroughJunction(t *testing.T) {
	launched := stubRun(t)
	root := t.TempDir()
	outside := t.TempDir()
	touch(t, filepath.Join(outside, "vcredist_x64.exe"))
	link := filepath.Join(root, "redist")
	//nolint:gosec // G204: test-only, fixed program and arguments, paths come from t.TempDir (invariant 33 concerns launcher binaries)
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}

	s := startedService(t, root)
	_, err := s.Install("g1", filepath.Join(link, "vcredist_x64.exe"))
	if uierr.Code(err) != "redist.outside_install" {
		t.Fatalf("err = %v", err)
	}
	if *launched != 0 {
		t.Fatal("installer launched through a junction")
	}
}
