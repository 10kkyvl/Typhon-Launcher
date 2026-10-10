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
