package platform

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func SaveRoots() ([]SaveRoot, error) {
	documents, err := knownFolder(windows.FOLDERID_Documents)
	if err != nil {
		return nil, fmt.Errorf("documents folder: %w", err)
	}
	savedGames, err := knownFolder(windows.FOLDERID_SavedGames)
	if err != nil {
		return nil, fmt.Errorf("saved games folder: %w", err)
	}
	roaming, err := knownFolder(windows.FOLDERID_RoamingAppData)
	if err != nil {
		return nil, fmt.Errorf("roaming appdata folder: %w", err)
	}
	local, err := knownFolder(windows.FOLDERID_LocalAppData)
	if err != nil {
		return nil, fmt.Errorf("local appdata folder: %w", err)
	}
	localLow, err := knownFolder(windows.FOLDERID_LocalAppDataLow)
	if err != nil {
		return nil, fmt.Errorf("locallow appdata folder: %w", err)
	}
	return []SaveRoot{
		{Path: filepath.Join(documents, "My Games"), Depth: 2},
		{Path: documents, Depth: 1},
		{Path: savedGames, Depth: 1},
		{Path: roaming, Depth: 2},
		{Path: local, Depth: 2},
		{Path: localLow, Depth: 2},
	}, nil
}

func knownFolder(id *windows.KNOWNFOLDERID) (string, error) {
	path, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", ErrEmptyPath
	}
	return path, nil
}
