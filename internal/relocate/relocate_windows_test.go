//go:build windows

package relocate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// unusedDriveTarget returns a path on a drive letter that does not exist,
// so platform.GetStorageInfo cannot resolve free space for it.
func unusedDriveTarget(t *testing.T) string {
	t.Helper()
	for _, letter := range "QXYZWVUT" {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); os.IsNotExist(err) {
			return root + `move-target`
		}
	}
	t.Skip("no unused drive letter available to force a storage-info failure")
	return ""
}

func TestMoveGameRefusesWhenFreeSpaceUnknown(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)
	target := unusedDriveTarget(t)

	if _, err := s.MoveGame(game.ID, target); !errors.Is(err, ErrFreeSpaceUnknown) {
		t.Fatalf("err = %v, want ErrFreeSpaceUnknown", err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "game.exe")); err != nil {
		t.Fatalf("source touched despite refusal: %v", err)
	}
}
