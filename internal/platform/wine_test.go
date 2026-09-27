package platform

import (
	"runtime"
	"testing"
)

func TestWineStatusRequiredOnlyOnDarwin(t *testing.T) {
	got := Wine()
	if got.Required != (runtime.GOOS == "darwin") {
		t.Fatalf("Required = %v on %s", got.Required, runtime.GOOS)
	}
	if !got.Required && (got.Installed || got.Version != "") {
		t.Fatalf("got %+v, want an empty status off darwin", got)
	}
}

func TestWineStatusVersionOnlyWhenInstalled(t *testing.T) {
	got := Wine()
	if !got.Installed && got.Version != "" {
		t.Fatalf("Version = %q while Installed = false", got.Version)
	}
}
