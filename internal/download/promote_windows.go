package download

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// renameExclusive uses MoveFileEx without MOVEFILE_REPLACE_EXISTING, unlike
// os.Rename, so a final file that appeared meanwhile is never overwritten.
func renameExclusive(from, to string) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	dst, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(src, dst, 0); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// Go opens files without FILE_SHARE_DELETE, so any reader the engine has open
// at that instant makes the rename fail until it closes the handle.
func isSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
