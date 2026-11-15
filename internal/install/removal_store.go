package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"typhon/internal/platform"
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
	// failed is set while the journal could not be read; late holds what it
	// held when it was read after all, for the sweep the startup skipped.
	failed bool
	late   []removalEntry
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
	entries, err := s.readLocked()
	if err != nil {
		s.failed = true
		return err
	}
	if s.failed {
		s.failed = false
		s.late = append([]removalEntry(nil), entries...)
	}
	s.entries, s.loaded = entries, true
	return nil
}

func (s *removalStore) readLocked() ([]removalEntry, error) {
	if s.dir == "" {
		return nil, fmt.Errorf("%w: config dir unknown", errRemovalsUnreadable)
	}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", errRemovalsUnreadable, s.path(), err)
	}
	var entries []removalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %w", errRemovalsUnreadable, s.path(), err)
	}
	return entries, nil
}

// takeLate hands over, once, the entries of a journal that was read only
// after an earlier read failed: the startup sweep went without them.
func (s *removalStore) takeLate() ([]removalEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.late == nil {
		return nil, false
	}
	late := s.late
	s.late = nil
	return late, true
}

// claim journals entry under the first staged name for its folder that is
// neither on disk nor in the journal, and returns that name. A journaled name
// may belong to a removal whose library half is still to be finished at the
// next start: staging under it would finish that removal under this one's game
// and choice, or this one under that one's.
func (s *removalStore) claim(entry removalEntry) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return "", err
	}
	staged, err := freeRemovalPath(entry.InstallDir, s.journaledLocked)
	if err != nil {
		return "", err
	}
	entry.Path = staged
	if err := s.appendLocked(entry); err != nil {
		return "", err
	}
	return staged, nil
}

func (s *removalStore) add(entry removalEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	if s.journaledLocked(entry.Path) {
		return fmt.Errorf("removal %s is already journaled", entry.Path)
	}
	return s.appendLocked(entry)
}

// journaledLocked compares as the file system does: on Windows two cards may
// spell one folder in different case.
func (s *removalStore) journaledLocked(path string) bool {
	for _, existing := range s.entries {
		if platform.SamePath(existing.Path, path) {
			return true
		}
	}
	return false
}

func (s *removalStore) appendLocked(entry removalEntry) error {
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
		return fmt.Errorf("%w: config dir unknown", errRemovalsUnreadable)
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
