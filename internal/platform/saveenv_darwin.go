//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"typhon/internal/wine"
)

func SaveEnvs() ([]SaveEnv, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	var bottles []wine.Bottle
	if rt, detectErr := wine.Detect(); detectErr == nil {
		manager := wine.NewManager(rt)
		list, listErr := manager.List()
		if listErr != nil {
			return nil, fmt.Errorf("list bottles: %w", listErr)
		}
		bottles = list
		if path, ok := manager.SharedBottlePath(); ok {
			bottles = append(bottles, wine.Bottle{Path: path, Shared: true})
		}
	}
	return saveEnvsFrom(home, bottles), nil
}

// Documents общий для всех бутылей: CrossOver делает его симлинком на
// настоящий ~/Documents (см. saveRootsFrom), поэтому токен {documents}
// живёт в одной хостовой среде, а не в каждой бутыли. Хостовая среда есть
// всегда, даже без бутылей: через неё раскрывается {game}.
func saveEnvsFrom(home string, bottles []wine.Bottle) []SaveEnv {
	envs := make([]SaveEnv, 0, len(bottles)+1)
	envs = append(envs, SaveEnv{TokenDocuments: filepath.Join(home, "Documents")})
	for _, bottle := range bottles {
		drive := filepath.Join(bottle.Path, "drive_c")
		user := filepath.Join(drive, "users", "crossover")
		env := SaveEnv{
			TokenUserProfile:     user,
			TokenAppData:         filepath.Join(user, "AppData", "Roaming"),
			TokenLocalAppData:    filepath.Join(user, "AppData", "Local"),
			TokenLocalAppDataLow: filepath.Join(user, "AppData", "LocalLow"),
			TokenSavedGames:      filepath.Join(user, "Saved Games"),
			TokenPublic:          filepath.Join(drive, "users", "Public"),
			TokenProgramData:     filepath.Join(drive, "ProgramData"),
		}
		if bottle.Shared {
			env[TokenSteam] = filepath.Join(drive, "Program Files (x86)", "Steam")
		}
		envs = append(envs, env)
	}
	return envs
}
