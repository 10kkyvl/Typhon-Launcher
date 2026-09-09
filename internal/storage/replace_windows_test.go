package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAtomicSurvivesConcurrentReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteAtomic(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	// os.Open on Windows shares read/write but not delete access.
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			if err := reader.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	result := make(chan error, 1)
	go func() { result <- WriteAtomic(path, []byte("new")) }()
	select {
	case err := <-result:
		t.Fatalf("replacement completed while reader held the file: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement did not finish after reader closed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("state = %s, want new", data)
	}
}
