package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"typhon/internal/storage"
)

type removalStore struct {
	mu    sync.Mutex
	dir   string
	paths []string
}

func newRemovalStore(dir string) *removalStore {
	return &removalStore{dir: dir}
}

func (s *removalStore) path() string {
	return filepath.Join(s.dir, "removals.json")
}

func (s *removalStore) load() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil, errors.New("removals path unavailable")
	}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		s.paths = nil
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read removals %s: %w", s.path(), err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("parse removals %s: %w", s.path(), err)
	}
	s.paths = paths
	return append([]string(nil), paths...), nil
}

func (s *removalStore) add(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.paths {
		if existing == path {
			return nil
		}
	}
	previous := s.paths
	s.paths = append(append([]string(nil), s.paths...), path)
	if err := s.saveLocked(); err != nil {
		s.paths = previous
		return err
	}
	return nil
}

func (s *removalStore) drop(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]string, 0, len(s.paths))
	for _, existing := range s.paths {
		if existing != path {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(s.paths) {
		return nil
	}
	previous := s.paths
	s.paths = kept
	if err := s.saveLocked(); err != nil {
		s.paths = previous
		return err
	}
	return nil
}

func (s *removalStore) saveLocked() error {
	if s.dir == "" {
		return errors.New("removals path unavailable")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.paths, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.path(), data)
}
