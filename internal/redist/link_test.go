package redist

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"typhon/internal/uierr"
)

const errPrivilegeNotHeld = syscall.Errno(1314)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if errors.Is(err, errPrivilegeNotHeld) || errors.Is(err, os.ErrPermission) {
		t.Skipf("symlinks need a privilege here: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func stubRun(t *testing.T) *int {
	t.Helper()
	launched := new(int)
	prev := run
	run = func(context.Context, string, []string, string) (int, error) {
		*launched++
		return 0, nil
	}
	t.Cleanup(func() { run = prev })
	return launched
}

func TestInstallRefusesThroughSymlinkedDirectory(t *testing.T) {
	launched := stubRun(t)
	root := t.TempDir()
	outside := t.TempDir()
	touch(t, filepath.Join(outside, "vcredist_x64.exe"))
	symlinkOrSkip(t, outside, filepath.Join(root, "redist"))

	s := startedService(t, root)
	_, err := s.Install("g1", filepath.Join(root, "redist", "vcredist_x64.exe"))
	if uierr.Code(err) != "redist.outside_install" {
		t.Fatalf("err = %v", err)
	}
	if *launched != 0 {
		t.Fatal("installer launched through a symlink")
	}
}

func TestInstallRefusesSymlinkedFile(t *testing.T) {
	launched := stubRun(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.exe")
	touch(t, outside)
	if err := os.MkdirAll(filepath.Join(root, "redist"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(root, "redist", "vcredist_x64.exe"))

	s := startedService(t, root)
	if _, err := s.Install("g1", filepath.Join(root, "redist", "vcredist_x64.exe")); uierr.Code(err) != "redist.outside_install" {
		t.Fatalf("err = %v", err)
	}
	if *launched != 0 {
		t.Fatal("installer launched through a symlink")
	}
}

func TestScanRejectsFileRoot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	touch(t, file)
	if _, err := Scan(context.Background(), file); !errors.Is(err, ErrRootNotDir) {
		t.Fatalf("file root: err = %v", err)
	}
}

func TestScanRejectsSymlinkRoot(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	symlinkOrSkip(t, real, link)
	if _, err := Scan(context.Background(), link); !errors.Is(err, ErrRootNotDir) {
		t.Fatalf("symlink root: err = %v", err)
	}
}

func TestCancelRunKeepsTheKillFailure(t *testing.T) {
	denied := errors.New("access denied")
	err := cancelRun(context.Canceled, func() error { return denied })
	if !errors.Is(err, ErrCancelFailed) || !errors.Is(err, denied) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("a failed terminate must not read as a clean cancellation")
	}
	if err := cancelRun(context.Canceled, func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("clean cancel: err = %v", err)
	}
}

func TestServiceMapsCancelFailure(t *testing.T) {
	prev := run
	run = func(context.Context, string, []string, string) (int, error) {
		return 0, cancelRun(context.Canceled, func() error { return errors.New("access denied") })
	}
	t.Cleanup(func() { run = prev })
	root := t.TempDir()
	path := filepath.Join(root, "redist", "vcredist_x64.exe")
	touch(t, path)
	s := startedService(t, root)
	if _, err := s.Install("g1", path); uierr.Code(err) != "redist.cancel_failed" {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceInstallFailureCarriesExitCode(t *testing.T) {
	prev := run
	run = func(context.Context, string, []string, string) (int, error) { return 1603, nil }
	t.Cleanup(func() { run = prev })
	root := t.TempDir()
	path := filepath.Join(root, "redist", "vcredist_x64.exe")
	touch(t, path)
	s := startedService(t, root)
	_, err := s.Install("g1", path)
	var exitErr *ExitError
	if uierr.Code(err) != "redist.failed" || !errors.As(err, &exitErr) || exitErr.Code != 1603 {
		t.Fatalf("err = %v", err)
	}
}
