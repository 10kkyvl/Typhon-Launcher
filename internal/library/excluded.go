package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/platform"
	"typhon/internal/storage"
)

const excludedName = "library-excluded.json"

func excludedPathFor(libraryPath string) (string, error) {
	if strings.TrimSpace(libraryPath) == "" {
		return "", errors.New("library path unavailable")
	}
	return filepath.Join(filepath.Dir(libraryPath), excludedName), nil
}

func loadExcluded(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read excluded %s: %w", path, err)
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse excluded %s: %w", path, err)
	}
	return keys, nil
}

func (s *Service) persistExcludedLocked() error {
	if s.excludedPath == "" {
		return errors.New("excluded path unavailable")
	}
	data, err := json.MarshalIndent(s.excluded, "", "  ")
	if err != nil {
		return err
	}
	if err := storage.WriteAtomic(s.excludedPath, data); err != nil {
		return fmt.Errorf("write excluded %s: %w", s.excludedPath, err)
	}
	return nil
}

func (s *Service) excludedLocked(dir string) (bool, error) {
	if strings.TrimSpace(dir) == "" || len(s.excluded) == 0 {
		return false, nil
	}
	key, err := platform.PathKey(dir)
	if err != nil {
		return false, fmt.Errorf("normalize %s: %w", dir, err)
	}
	for _, existing := range s.excluded {
		if existing == key {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) excludeLocked(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	key, err := platform.PathKey(dir)
	if err != nil {
		return fmt.Errorf("normalize %s: %w", dir, err)
	}
	for _, existing := range s.excluded {
		if existing == key {
			return nil
		}
	}
	previous := s.excluded
	s.excluded = append(append([]string(nil), s.excluded...), key)
	if err := s.persistExcludedLocked(); err != nil {
		s.excluded = previous
		return err
	}
	return nil
}

func (s *Service) allowLocked(dir string) error {
	if strings.TrimSpace(dir) == "" || len(s.excluded) == 0 {
		return nil
	}
	key, err := platform.PathKey(dir)
	if err != nil {
		return fmt.Errorf("normalize %s: %w", dir, err)
	}
	kept := make([]string, 0, len(s.excluded))
	for _, existing := range s.excluded {
		if existing != key {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(s.excluded) {
		return nil
	}
	previous := s.excluded
	s.excluded = kept
	if err := s.persistExcludedLocked(); err != nil {
		s.excluded = previous
		return err
	}
	return nil
}
