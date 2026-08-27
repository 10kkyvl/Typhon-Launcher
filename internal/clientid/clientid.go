package clientid

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/settings"
	"typhon/internal/storage"

	"github.com/google/uuid"
)

type Identity struct {
	InstallationID string
	SessionID      string
}

type installationFile struct {
	InstallationID string `json:"installationId"`
}

func Load() (Identity, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return Identity{}, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return Identity{}, errors.New("config dir unavailable")
	}
	return LoadAt(filepath.Join(dir, "installation.json"))
}

func LoadAt(path string) (Identity, error) {
	if path == "" {
		return Identity{}, errors.New("installation id path unavailable")
	}
	installationID, err := loadOrCreate(path)
	if err != nil {
		return Identity{}, err
	}
	sessionID, err := uuid.NewRandom()
	if err != nil {
		return Identity{}, fmt.Errorf("generate session id: %w", err)
	}
	return Identity{InstallationID: installationID, SessionID: sessionID.String()}, nil
}

func loadOrCreate(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return create(path)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	var f installationFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	parsed, err := uuid.Parse(f.InstallationID)
	if err != nil {
		return "", fmt.Errorf("invalid installation id in %s: %w", path, err)
	}
	return parsed.String(), nil
}

func create(path string) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate installation id: %w", err)
	}
	data, err := json.Marshal(installationFile{InstallationID: id.String()})
	if err != nil {
		return "", fmt.Errorf("encode installation id: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := storage.WriteAtomic(path, data); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return id.String(), nil
}
