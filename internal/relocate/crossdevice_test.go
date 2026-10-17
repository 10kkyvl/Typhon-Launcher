package relocate

import (
	"os"
	"testing"
)

func forceCopyPath(t *testing.T) {
	t.Helper()
	prev := renameDir
	renameDir = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errCrossDevice}
	}
	t.Cleanup(func() { renameDir = prev })
}
