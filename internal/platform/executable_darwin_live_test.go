//go:build darwin && !devmock

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func TestSelectGameExecutableRejectsMissingDirectory(t *testing.T) {
	_, err := SelectGameExecutable("Pick", filepath.Join(t.TempDir(), "missing"), "")
	if code := uierr.Code(err); code != "library.no_install_dir" {
		t.Fatalf("code = %q, want library.no_install_dir; err=%v", code, err)
	}
}

// TestSelectGameExecutableLive is opt-in because it opens a real modal window
// in CrossOver. It does not change the library or launch the selected game.
func TestSelectGameExecutableLive(t *testing.T) {
	dir := os.Getenv("TYPHON_LIVE_PICKER_DIR")
	want := os.Getenv("TYPHON_LIVE_PICKER_FILE")
	if dir == "" || want == "" {
		t.Skip("set TYPHON_LIVE_PICKER_DIR and TYPHON_LIVE_PICKER_FILE")
	}
	got, err := SelectGameExecutable("Typhon executable picker test", dir, want)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("selected %q, want %q", got, want)
	}
}
