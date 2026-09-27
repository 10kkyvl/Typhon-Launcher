package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExplorerPathIsAbsolute closes finding 6 (open.go): OpenFolder used to
// run exec.Command("explorer.exe", path), a bare name resolved through PATH
// at runtime (invariant 33). explorerPath must hand back an absolute,
// verified path instead.
func TestExplorerPathIsAbsolute(t *testing.T) {
	path, err := explorerPath()
	if err != nil {
		t.Fatalf("explorerPath: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("path = %q, want an absolute path", path)
	}
	if !strings.EqualFold(filepath.Base(path), "explorer.exe") {
		t.Fatalf("path = %q, want it to end in explorer.exe", path)
	}
}
