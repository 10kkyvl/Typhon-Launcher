//go:build windows

package install

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestReparsePointByAttributes(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(dir, "file.bin")
	mkText(t, file, "1")
	link := filepath.Join(dir, "lnk")
	if err := makeDirLink(link, outside); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "regular file", path: file},
		{name: "directory", path: outside},
		{name: "junction", path: link, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := os.Lstat(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := reparsePoint(info)
			if err != nil {
				t.Fatalf("reparsePoint: %v", err)
			}
			if got != tc.want {
				t.Fatalf("reparsePoint(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestSharingViolationIsReportedAsBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.bin")
	mkText(t, path, "held by another program")
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, syncErr := syncAndCountLinks(path, 0o600)
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(syncErr, windows.ERROR_SHARING_VIOLATION) || !osFileBusy(syncErr) {
		t.Fatalf("err = %v, want a sharing violation that osFileBusy recognises", syncErr)
	}
	if osFileBusy(fs.ErrNotExist) || osFileBusy(fs.ErrPermission) || osFileBusy(nil) {
		t.Fatal("osFileBusy treats an error that no wait can fix as a transient one")
	}
	if links, err := syncAndCountLinks(path, 0o600); err != nil || links != 1 {
		t.Fatalf("after the handle is closed: links = %d, err = %v", links, err)
	}
}

func TestSyncReadOnlyFileOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.bin")
	mkText(t, path, "read only")
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore mode: %v", err)
		}
	})
	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		if cerr := f.Close(); cerr != nil {
			t.Fatal(cerr)
		}
		t.Fatal("precondition: a read-only file opened for writing")
	}
	links, err := syncAndCountLinks(path, 0o400)
	if err != nil {
		t.Fatalf("syncAndCountLinks: %v", err)
	}
	if links != 1 {
		t.Fatalf("links = %d, want 1", links)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatalf("mode = %v, want the read-only attribute back", info.Mode())
	}
}
