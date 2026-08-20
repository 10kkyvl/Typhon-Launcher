package install

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
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

func (s *store) load() []Installation {
	if s.dir == "" {
		return nil
	}
	data, err := os.ReadFile(s.listPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		slog.Error("read installations", "path", s.listPath(), "error", err)
		return nil
	}
	var items []Installation
	if err := json.Unmarshal(data, &items); err != nil {
		slog.Error("parse installations", "path", s.listPath(), "error", err)
		return nil
	}
	return items
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
	return writeAtomic(s.listPath(), data)
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Error("write file", "path", tmp, "error", err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		slog.Error("replace file", "path", path, "error", err)
		return err
	}
	return nil
}
