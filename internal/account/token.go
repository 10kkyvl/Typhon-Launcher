package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/settings"
)

var ErrNoToken = errors.New("no api token")

const maxAuthFileSize = 1 << 16

type authFile struct {
	Token string `json:"token"`
}

func Token() (string, error) {
	if env := os.Getenv("TYPHON_API_TOKEN"); env != "" {
		return env, nil
	}

	dir, err := settings.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	path := filepath.Join(dir, "auth.json")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoToken
	}
	if err != nil {
		return "", fmt.Errorf("open auth file %s: %w", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Debug("close auth file", "error", err)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(f, maxAuthFileSize))
	if err != nil {
		return "", fmt.Errorf("read auth file %s: %w", path, err)
	}

	var parsed authFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("parse auth file %s: %w", path, err)
	}

	if parsed.Token == "" {
		return "", ErrNoToken
	}
	return parsed.Token, nil
}
