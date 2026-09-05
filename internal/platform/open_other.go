//go:build !windows

package platform

import "errors"

// openFolderWindows существует только для того, чтобы open.go компилировался
// на всех ОС: ветка "windows" в OpenFolder на этой платформе не исполняется.
func openFolderWindows(string) error {
	return errors.New("explorer.exe is only available on windows")
}
