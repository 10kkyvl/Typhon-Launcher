package sources

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

const (
	sourcesVersion  = 1
	releasesVersion = 1
)

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) sourcesPath() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, "sources.json")
}

func (s *store) releasesPath(sourceID string) string {
	if s.dir == "" || sourceID == "" {
		return ""
	}
	return filepath.Join(s.dir, "releases", sourceID+".json")
}

func (s *store) loadSources() []Source {
	path := s.sourcesPath()
	if path == "" {
		return nil
	}
	var list []Source
	if err := storage.Load(path, sourcesVersion, nil, &list); err != nil {
		slog.Error("load sources", "error", err)
		return nil
	}
	return list
}

func (s *store) saveSources(list []Source) error {
	path := s.sourcesPath()
	if path == "" {
		return errors.New("sources path unavailable")
	}
	return storage.Save(path, sourcesVersion, list)
}

func (s *store) loadReleases(sourceID string) []Release {
	path := s.releasesPath(sourceID)
	if path == "" {
		return nil
	}
	var list []Release
	if err := storage.Load(path, releasesVersion, nil, &list); err != nil {
		slog.Error("load releases", "source", sourceID, "error", err)
		return nil
	}
	return list
}

func (s *store) saveReleases(sourceID string, list []*Release) error {
	path := s.releasesPath(sourceID)
	if path == "" {
		return errors.New("releases path unavailable")
	}
	flat := make([]Release, 0, len(list))
	for _, r := range list {
		flat = append(flat, *r)
	}
	return storage.Save(path, releasesVersion, flat)
}

func (s *store) removeReleases(sourceID string) {
	path := s.releasesPath(sourceID)
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove releases file", "source", sourceID, "error", err)
	}
}
