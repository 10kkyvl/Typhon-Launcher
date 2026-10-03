//go:build windows

package install

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var userShortcutFolders = []*windows.KNOWNFOLDERID{
	windows.FOLDERID_Desktop,
	windows.FOLDERID_Programs,
}

// Общие каталоги пишутся только с правами администратора: установщик,
// поднятый через UAC, кладёт ярлыки туда, а лаунчер их уже не удалит. Их
// убирает повышенный воркер (shortcuts_worker.go), поэтому список вычисляется
// заново на его стороне и из spec не приходит.
var sharedShortcutFolders = []*windows.KNOWNFOLDERID{
	windows.FOLDERID_PublicDesktop,
	windows.FOLDERID_CommonPrograms,
}

func shortcutRoots() ([]string, error) {
	user, err := knownFolderPaths(userShortcutFolders)
	if err != nil {
		return nil, err
	}
	shared, err := sharedShortcutRoots()
	if err != nil {
		return nil, err
	}
	return append(user, shared...), nil
}

func sharedShortcutRoots() ([]string, error) {
	return knownFolderPaths(sharedShortcutFolders)
}

func knownFolderPaths(ids []*windows.KNOWNFOLDERID) ([]string, error) {
	roots := make([]string, 0, len(ids))
	for _, id := range ids {
		path, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
		if err != nil {
			return nil, fmt.Errorf("known folder %v: %w", id, err)
		}
		roots = append(roots, path)
	}
	return roots, nil
}
