package gamemode

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

const (
	markerVersion = 1
	markerName    = "gamemode.json"
)

type powerMark struct {
	Previous string `json:"previous"`
	Applied  string `json:"applied"`
}

type marker struct {
	Power *powerMark `json:"power,omitempty"`
	Hide  bool       `json:"hide,omitempty"`
}

func (m marker) empty() bool {
	return m.Power == nil && !m.Hide
}

type store struct {
	path string
}

func newStore(dir string) *store {
	return &store{path: filepath.Join(dir, markerName)}
}

func (st *store) load() (marker, bool, error) {
	var m marker
	if err := storage.Load(st.path, markerVersion, nil, &m); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return marker{}, false, nil
		}
		return marker{}, false, err
	}
	return m, true, nil
}

func (st *store) save(m marker) error {
	if m.empty() {
		if err := os.Remove(st.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove game mode marker: %w", err)
		}
		return nil
	}
	if err := storage.Save(st.path, markerVersion, m); err != nil {
		return fmt.Errorf("write game mode marker: %w", err)
	}
	return nil
}
