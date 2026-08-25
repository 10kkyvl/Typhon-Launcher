//go:build windows

package install

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Общие каталоги пишутся только с правами администратора: установщик,
// поднятый через UAC, кладёт ярлыки туда, а лаунчер их уже не удалит.
var shortcutFolders = []*windows.KNOWNFOLDERID{
	windows.FOLDERID_Desktop,
	windows.FOLDERID_PublicDesktop,
	windows.FOLDERID_Programs,
	windows.FOLDERID_CommonPrograms,
}

func shortcutRoots() ([]string, error) {
	roots := make([]string, 0, len(shortcutFolders))
	for _, id := range shortcutFolders {
		path, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
		if err != nil {
			return nil, fmt.Errorf("known folder %v: %w", id, err)
		}
		roots = append(roots, path)
	}
	return roots, nil
}
