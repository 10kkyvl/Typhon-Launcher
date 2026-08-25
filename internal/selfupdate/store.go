package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"typhon/internal/storage"
)

type Store struct {
	mu       sync.Mutex
	path     string
	readOnly bool
}

func NewStore(configDir string) (*Store, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "state.json")}, nil
}

func (s *Store) Load() (stored, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var v stored
	err := storage.Load(s.path, StateVersion, nil, &v)
	if errors.Is(err, fs.ErrNotExist) {
		return stored{}, nil
	}
	if err != nil {
		s.readOnly = true
		return stored{}, fmt.Errorf("load selfupdate state: %w", err)
	}
	return v, nil
}

func (s *Store) Save(v stored) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return ErrReadOnly
	}
	if err := storage.Save(s.path, StateVersion, v); err != nil {
		return fmt.Errorf("save selfupdate state: %w", err)
	}
	return nil
}
