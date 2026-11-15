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
	"typhon/internal/uierr"
)

var errRemovalsUnreadable = uierr.New("install.removals_unreadable", "журнал незавершённых удалений не читается, удалять файлы игр нельзя, пока его не прочитать")

// removalEntry is a removal whose files may be on their way out. Path is the
// staged folder; GameID, InstallDir and KeepInLibrary let the startup sweep
// finish the library half as well. An older launcher wrote bare paths, which
// carry no game.
type removalEntry struct {
	Path          string `json:"path"`
	GameID        string `json:"gameId,omitempty"`
	InstallDir    string `json:"installDir,omitempty"`
	KeepInLibrary bool   `json:"keepInLibrary"`
}

func (e *removalEntry) UnmarshalJSON(data []byte) error {
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		*e = removalEntry{Path: path}
		return nil
	}
	type plain removalEntry
	var entry plain
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	*e = removalEntry(entry)
	return nil
}

type removalStore struct {
	mu      sync.Mutex
	dir     string
	entries []removalEntry
	loaded  bool
}

func newRemovalStore(dir string) *removalStore {
	return &removalStore{dir: dir}
}

func (s *removalStore) path() string {
	return filepath.Join(s.dir, "removals.json")
}

func (s *removalStore) load() ([]removalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	return append([]removalEntry(nil), s.entries...), nil
}

// ready says whether a removal may be journaled now.
func (s *removalStore) ready() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// loadLocked reads the journal before the first write, whoever comes first: the
// startup sweep or a removal. Only a missing file is an empty journal. One that
// cannot be read or parsed is never written over, because it may hold the only
// record of a killed removal; it is read again on the next try instead.
func (s *removalStore) loadLocked() error {
	if s.loaded {
		return nil
	}
	if s.dir == "" {
		return errors.New("removals path unavailable")
	}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		s.entries, s.loaded = nil, true
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read %s: %w", errRemovalsUnreadable, s.path(), err)
	}
	var entries []removalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("%w: parse %s: %w", errRemovalsUnreadable, s.path(), err)
	}
	s.entries, s.loaded = entries, true
	return nil
}

func (s *removalStore) add(entry removalEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	for _, existing := range s.entries {
		if existing.Path == entry.Path {
			return nil
		}
	}
	previous := s.entries
	s.entries = append(append([]removalEntry(nil), s.entries...), entry)
	if err := s.saveLocked(); err != nil {
		s.entries = previous
		return err
	}
	return nil
}

func (s *removalStore) drop(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	kept := make([]removalEntry, 0, len(s.entries))
	for _, existing := range s.entries {
		if existing.Path != path {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(s.entries) {
		return nil
	}
	previous := s.entries
	s.entries = kept
	if err := s.saveLocked(); err != nil {
		s.entries = previous
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
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.path(), data)
}
