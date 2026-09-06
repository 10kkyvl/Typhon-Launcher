package procs

import (
	"context"
	"runtime"
	"testing"

	"typhon/internal/devmock"
)

// Перечисление процессов поддержано там, где есть чему их перечислять:
// Windows нативно, macOS через бутыли CrossOver, devmock — фейковым реестром.
func TestSupportedMatchesGOOS(t *testing.T) {
	want := runtime.GOOS == "windows" || runtime.GOOS == "darwin" || devmock.Enabled
	if got := Supported(); got != want {
		t.Fatalf("Supported() = %v, want %v for GOOS=%s", got, want, runtime.GOOS)
	}
}

func TestListOnUnsupportedPlatform(t *testing.T) {
	if Supported() {
		t.Skip("this case is exercised on platforms without process enumeration only")
	}
	got, err := List(context.Background())
	if err == nil {
		t.Fatalf("List() on %s returned nil error, want an error", runtime.GOOS)
	}
	if got != nil {
		t.Fatalf("List() on %s returned %v processes alongside an error, want nil", runtime.GOOS, got)
	}
}
