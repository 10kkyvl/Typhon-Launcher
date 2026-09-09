package library

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRegressionRelocateLeavesOldUninstaller(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	g, old := addRelocateGame(t, s)
	s.mu.Lock()
	s.games[0].Uninstall = Uninstall{Command: filepath.Join(old, "unins000.exe"), QuietCommand: filepath.Join(old, "unins000.exe") + " /S"}
	s.mu.Unlock()
	next, err := s.Relocate(g.ID, filepath.Join(t.TempDir(), "new"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(next.Uninstall.Command, old) || strings.Contains(next.Uninstall.QuietCommand, old) {
		t.Fatal("not reproduced")
	}
	t.Log("Regression: uninstaller points at old directory after move")
}
