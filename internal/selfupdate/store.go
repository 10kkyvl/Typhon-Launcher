package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"typhon/internal/storage"
	"typhon/internal/version"
)

// storedFile is what state.json holds. A stable launcher fails its start on an
// availableVersion it cannot parse, and a beta version is exactly that, so an
// update to a beta lives under Beta where older launchers do not look.
type storedFile struct {
	stored
	Beta *stored `json:"beta,omitempty"`
}

func toFile(v stored) storedFile {
	if !version.IsPrerelease(version.Parse(v.AvailableVersion)) {
		return storedFile{stored: v}
	}
	payload := v
	payload.SkippedVersion = ""
	return storedFile{
		stored: stored{CheckedAt: v.CheckedAt, SkippedVersion: v.SkippedVersion},
		Beta:   &payload,
	}
}

func (f storedFile) fromFile() stored {
	if f.Beta == nil {
		return f.stored
	}
	v := *f.Beta
	v.CheckedAt = f.CheckedAt
	v.SkippedVersion = f.SkippedVersion
	return v
}

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

	var f storedFile
	err := storage.Load(s.path, StateVersion, nil, &f)
	if errors.Is(err, fs.ErrNotExist) {
		return stored{}, nil
	}
	if err != nil {
		s.readOnly = true
		return stored{}, fmt.Errorf("load selfupdate state: %w", err)
	}
	return f.fromFile(), nil
}

func (s *Store) Save(v stored) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return ErrReadOnly
	}
	if err := storage.Save(s.path, StateVersion, toFile(v)); err != nil {
		return fmt.Errorf("save selfupdate state: %w", err)
	}
	return nil
}
