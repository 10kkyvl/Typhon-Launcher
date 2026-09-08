//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"typhon/internal/wine"
)

// SaveRoots на macOS складывается из двух классов корней. Внутрибутыльные —
// Saved Games и AppData — свои у каждой игры. Documents общий: CrossOver
// делает его симлинком на настоящий ~/Documents, поэтому туда пишут все
// бутыли сразу, и добавлять его на каждый бутыль незачем.
func SaveRoots() ([]SaveRoot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	var bottles []wine.Bottle
	if rt, detectErr := wine.Detect(); detectErr == nil {
		manager := wine.NewManager(rt)
		if list, listErr := manager.List(); listErr == nil {
			bottles = list
		}
		// Общий бутыль со Steam в List не входит — он пользовательский и
		// нашей метки не имеет, — но игры, запущенные в нём, пишут сейвы
		// именно туда, и без него они бы не нашлись.
		if path, ok := manager.SharedBottlePath(); ok {
			bottles = append(bottles, wine.Bottle{Path: path, Shared: true})
		}
	}
	return saveRootsFrom(home, bottles), nil
}

func saveRootsFrom(home string, bottles []wine.Bottle) []SaveRoot {
	roots := make([]SaveRoot, 0, len(bottles)*4+2)
	for _, bottle := range bottles {
		user := filepath.Join(bottle.Path, "drive_c", "users", "crossover")
		roots = append(roots,
			SaveRoot{Path: filepath.Join(user, "Saved Games"), Depth: 1},
			SaveRoot{Path: filepath.Join(user, "AppData", "Roaming"), Depth: 2},
			SaveRoot{Path: filepath.Join(user, "AppData", "Local"), Depth: 2},
			SaveRoot{Path: filepath.Join(user, "AppData", "LocalLow"), Depth: 2},
		)
	}
	return append(roots,
		SaveRoot{Path: filepath.Join(home, "Documents", "My Games"), Depth: 2},
		SaveRoot{Path: filepath.Join(home, "Documents"), Depth: 1},
	)
}
