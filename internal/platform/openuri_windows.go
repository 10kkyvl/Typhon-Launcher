//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func OpenURI(uri string) error {
	handle, err := ShellExecute("open", uri, "", "", windows.SW_SHOWNORMAL)
	if err != nil {
		return fmt.Errorf("open %s: %w", uri, err)
	}
	if handle != 0 {
		if err := windows.CloseHandle(handle); err != nil {
			return fmt.Errorf("close handle for %s: %w", uri, err)
		}
	}
	return nil
}
