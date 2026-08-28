package procs

import (
	"context"
	"runtime"
	"testing"
)

func TestSupportedMatchesGOOS(t *testing.T) {
	want := runtime.GOOS == "windows"
	if got := Supported(); got != want {
		t.Fatalf("Supported() = %v, want %v for GOOS=%s", got, want, runtime.GOOS)
	}
}

func TestListOnUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this case is exercised on non-Windows platforms only")
	}
	got, err := List(context.Background())
	if err == nil {
		t.Fatalf("List() on %s returned nil error, want an error", runtime.GOOS)
	}
	if got != nil {
		t.Fatalf("List() on %s returned %v processes alongside an error, want nil", runtime.GOOS, got)
	}
}
