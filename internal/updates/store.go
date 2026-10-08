package updates

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"typhon/internal/storage"
)

const (
	updatesVersion   = 1
	historyVersion   = 1
	rollbackVersion  = 1
	verifyVersion    = 1
	manifestsVersion = 1
	journalVersion   = 1

	maxHistory = 200
)

var errStoreUnloaded = errors.New("saved updates state was not loaded, saving is refused")

type store struct {
	dir string

	mu      sync.Mutex
	blocked error
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

// block makes every save fail until it is called with nil. A service whose
// state could not be read holds an empty picture of it, and the first save of
// that picture would replace the file it failed to read (invariant 3).
func (s *store) block(cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cause == nil {
		s.blocked = nil
		return
	}
	s.blocked = fmt.Errorf("%w: %w", errStoreUnloaded, cause)
}

func (s *store) writable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocked
}

func (s *store) save(name string, version int, data any) error {
	if err := s.writable(); err != nil {
		return err
	}
	return storage.Save(s.path(name), version, data)
}

type loadedState struct {
	updates       []Update
	verifications []VerifyState
	rollbacks     []Rollback
	history       []UpdateHistory
	journals      []SwapJournal
}

func (s *store) loadAll() (loadedState, error) {
	var state loadedState
	var err error
	if state.updates, err = s.loadUpdates(); err != nil {
		return loadedState{}, err
	}
	if state.verifications, err = s.loadVerifications(); err != nil {
		return loadedState{}, err
	}
	if state.rollbacks, err = s.loadRollbacks(); err != nil {
		return loadedState{}, err
	}
	if state.history, err = s.loadHistory(); err != nil {
		return loadedState{}, err
	}
	if state.journals, err = s.loadJournals(); err != nil {
		return loadedState{}, err
	}
	return state, nil
}

func (s *store) path(name string) string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, name)
}

func (s *store) manifestPath(gameID string) string {
	if s.dir == "" || gameID == "" {
		return ""
	}
	return filepath.Join(s.dir, "manifests", gameID+".json")
}

func load[T any](path string, version int, label string) ([]T, error) {
	if path == "" {
		return nil, fmt.Errorf("%s path unavailable", label)
	}
	var list []T
	err := storage.Load(path, version, nil, &list)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", label, err)
	}
	return list, nil
}

func (s *store) loadUpdates() ([]Update, error) {
	return load[Update](s.path("updates.json"), updatesVersion, "updates")
}

func (s *store) saveUpdates(list []Update) error {
	return s.save("updates.json", updatesVersion, list)
}

func (s *store) loadHistory() ([]UpdateHistory, error) {
	return load[UpdateHistory](s.path("update_history.json"), historyVersion, "update history")
}

func (s *store) saveHistory(list []UpdateHistory) error {
	if len(list) > maxHistory {
		list = list[len(list)-maxHistory:]
	}
	return s.save("update_history.json", historyVersion, list)
}

func (s *store) loadRollbacks() ([]Rollback, error) {
	return load[Rollback](s.path("rollbacks.json"), rollbackVersion, "rollbacks")
}

func (s *store) saveRollbacks(list []Rollback) error {
	return s.save("rollbacks.json", rollbackVersion, list)
}

func (s *store) loadJournals() ([]SwapJournal, error) {
	return load[SwapJournal](s.path("journal.json"), journalVersion, "journal")
}

func (s *store) saveJournals(list []SwapJournal) error {
	return s.save("journal.json", journalVersion, list)
}

func (s *store) loadVerifications() ([]VerifyState, error) {
	return load[VerifyState](s.path("verify.json"), verifyVersion, "verifications")
}

func (s *store) saveVerifications(list []VerifyState) error {
	return s.save("verify.json", verifyVersion, list)
}

func (s *store) loadManifest(gameID string) (FileManifest, bool, error) {
	path := s.manifestPath(gameID)
	if path == "" {
		return FileManifest{}, false, errors.New("manifest path unavailable")
	}
	var manifest FileManifest
	err := storage.Load(path, manifestsVersion, nil, &manifest)
	if errors.Is(err, fs.ErrNotExist) {
		return FileManifest{}, false, nil
	}
	if err != nil {
		return FileManifest{}, false, fmt.Errorf("load manifest %s: %w", gameID, err)
	}
	if len(manifest.Entries) == 0 {
		return FileManifest{}, false, nil
	}
	return manifest, true, nil
}

func (s *store) saveManifest(manifest FileManifest) error {
	path := s.manifestPath(manifest.GameID)
	if path == "" {
		return errors.New("manifest path unavailable")
	}
	return storage.Save(path, manifestsVersion, manifest)
}

func (s *store) removeManifest(gameID string) {
	path := s.manifestPath(gameID)
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove manifest", "game", gameID, "error", err)
	}
}
