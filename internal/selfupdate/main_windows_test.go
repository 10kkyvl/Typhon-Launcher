//go:build windows

package selfupdate

import (
	"log/slog"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestMain keeps the package off the real installation record. ServiceStartup
// writes the launcher's own directory there, and under a test binary that
// directory is a temporary build path: without this, running the suite would
// point the next installer run at a directory that no longer exists.
func TestMain(m *testing.M) {
	installDirKey = `Software\Typhon-test-suite`
	code := m.Run()
	if err := registry.DeleteKey(registry.CURRENT_USER, installDirKey); err != nil && !os.IsNotExist(err) {
		slog.Error("delete test install dir key", "key", installDirKey, "error", err)
		code = 1
	}
	os.Exit(code)
}
