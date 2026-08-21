package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) listPath() string {
	return filepath.Join(s.dir, "installations.json")
}

func (s *store) load() ([]Installation, error) {
	if s.dir == "" {
		return nil, errors.New("installations path unavailable")
	}
	data, err := os.ReadFile(s.listPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read installations %s: %w", s.listPath(), err)
	}
	var items []Installation
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse installations %s: %w", s.listPath(), err)
	}
	return items, nil
}

func (s *store) save(items []Installation) error {
	if s.dir == "" {
		return errors.New("installations path unavailable")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.listPath(), data)
}
