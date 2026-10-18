package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWriteAtomicGivesUpWhileDestinationStaysHeld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteAtomic(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	// os.Open on Windows shares read/write but not delete access, so the
	// rename onto the file keeps failing for as long as this handle lives.
	holder, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			if err := holder.Close(); err != nil {
				t.Error(err)
			}
		}
	}()

	result := make(chan error, 1)
	go func() { result <- WriteAtomic(path, []byte("new")) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("replacement succeeded while the destination was held open")
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Fatalf("error = %v, want the sharing violation that blocked the rename", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the retry loop is not bounded")
	}

	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	released = true
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("state = %q, want the old content untouched", got)
	}
	for _, name := range dirNames(t, dir) {
		if strings.HasPrefix(name, ".tmp-") {
			t.Fatalf("temp file left behind: %s", name)
		}
	}
}

func holdExclusive(t *testing.T, path string) windows.Handle {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestOpenReadWaitsOutBriefExclusiveLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := WriteAtomic(path, []byte("spec")); err != nil {
		t.Fatal(err)
	}
	h := holdExclusive(t, path)
	if _, err := os.Open(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("setup: plain open err = %v, want a sharing violation", err)
	}
	released := make(chan error, 1)
	time.AfterFunc(40*time.Millisecond, func() { released <- windows.CloseHandle(h) })

	f, err := OpenRead(path)
	if err != nil {
		t.Fatalf("OpenRead: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
}

func TestOpenReadGivesUpWhileLockStays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := WriteAtomic(path, []byte("spec")); err != nil {
		t.Fatal(err)
	}
	h := holdExclusive(t, path)
	t.Cleanup(func() {
		if err := windows.CloseHandle(h); err != nil {
			t.Error(err)
		}
	})
	if _, err := OpenRead(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("err = %v, want a sharing violation after the retries", err)
	}
}
