//go:build darwin

package instance

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLockFirstInstanceIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.lock")
	first, err := lockFirstInstance(path)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := lockFirstInstance(path); !errors.Is(err, ErrNotFirst) {
		t.Fatalf("second lock error = %v, want ErrNotFirst", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first lock: %v", err)
	}
	again, err := lockFirstInstance(path)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Fatalf("close second lock: %v", err)
	}
}

func TestTakeoverHoldsTheLockAndReleasesItOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.lock")
	prev := lockPath
	lockPath = func(string) string { return path }
	t.Cleanup(func() { lockPath = prev })

	l, err := Takeover(t.Context(), "id", func([]string) {})
	if err != nil {
		t.Fatalf("Takeover() error = %v", err)
	}
	if _, err := lockFirstInstance(path); !errors.Is(err, ErrNotFirst) {
		t.Fatalf("lock while taken: error = %v, want ErrNotFirst", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	f, err := lockFirstInstance(path)
	if err != nil {
		t.Fatalf("lock after Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close lock: %v", err)
	}
}
