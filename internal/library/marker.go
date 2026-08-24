package library

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"
)

const (
	MarkerName    = ".typhon-install.json"
	markerVersion = 1
)

var errMarkerEmpty = errors.New("метка установки не содержит идентификатора")

type Marker struct {
	GameID          string    `json:"gameId"`
	Title           string    `json:"title"`
	Executable      string    `json:"executable,omitempty"`
	Version         string    `json:"version,omitempty"`
	VersionSource   string    `json:"versionSource,omitempty"`
	ReleaseID       string    `json:"releaseId,omitempty"`
	SourceID        string    `json:"sourceId,omitempty"`
	CanonicalGameID string    `json:"canonicalGameId,omitempty"`
	InstallType     string    `json:"installType,omitempty"`
	Owned           bool      `json:"owned,omitempty"`
	Uninstall       Uninstall `json:"uninstall,omitzero"`
	InstalledAt     time.Time `json:"installedAt"`
}

func MarkerPath(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errEmptyInstallDir
	}
	return filepath.Join(dir, MarkerName), nil
}

func WriteMarker(dir string, m Marker) error {
	path, err := MarkerPath(dir)
	if err != nil {
		return err
	}
	if m.GameID == "" {
		return errMarkerEmpty
	}
	if err := storage.Save(path, markerVersion, m); err != nil {
		return fmt.Errorf("write install marker %s: %w", path, err)
	}
	return nil
}

func ReadMarker(dir string) (Marker, error) {
	path, err := MarkerPath(dir)
	if err != nil {
		return Marker{}, err
	}
	var m Marker
	if err := storage.Load(path, markerVersion, nil, &m); err != nil {
		return Marker{}, err
	}
	if m.GameID == "" {
		return Marker{}, fmt.Errorf("%s: %w", path, errMarkerEmpty)
	}
	return m, nil
}

func markerFor(game Game) Marker {
	executable := ""
	if game.Executable != "" && game.InstallDir != "" {
		rel, err := filepath.Rel(game.InstallDir, game.Executable)
		if err == nil && filepath.IsLocal(rel) {
			executable = filepath.ToSlash(rel)
		}
	}
	return Marker{
		GameID:          game.ID,
		Title:           game.Title,
		Executable:      executable,
		Version:         game.Version,
		VersionSource:   game.VersionSource,
		ReleaseID:       game.ReleaseID,
		SourceID:        game.SourceID,
		CanonicalGameID: game.CanonicalGameID,
		InstallType:     game.InstallType,
		Owned:           game.Owned,
		Uninstall:       game.Uninstall,
		InstalledAt:     game.InstalledAt,
	}
}

// MarkerExecutable отдаёт абсолютный путь исполняемого файла для каталога, в
// котором метка лежит сейчас: каталог могли переименовать или перенести.
func (m Marker) MarkerExecutable(dir string) string {
	if m.Executable == "" || dir == "" {
		return ""
	}
	rel := filepath.FromSlash(m.Executable)
	if !filepath.IsLocal(rel) {
		return ""
	}
	return filepath.Join(dir, rel)
}
