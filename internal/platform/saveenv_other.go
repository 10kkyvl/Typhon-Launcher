//go:build !windows && !darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func SaveEnvs() ([]SaveEnv, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	return []SaveEnv{{
		TokenUserProfile: home,
		TokenDocuments:   filepath.Join(home, "Documents"),
	}}, nil
}
