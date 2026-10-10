//go:build !windows

package download

import (
	"errors"
	"io/fs"
	"os"
)

// POSIX rename replaces its target, so the target is checked first; the window
// between the check and the rename is not closed here.
func renameExclusive(from, to string) error {
	_, err := os.Lstat(to)
	switch {
	case err == nil:
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	return os.Rename(from, to)
}

// Only Windows refuses to rename a file that another handle has open.
func isSharingViolation(error) bool {
	return false
}
