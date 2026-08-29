package accountsync

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"typhon/internal/storage"
)

const stateVersion = 1

type gameState struct {
	DeviceSeconds int64 `json:"deviceSeconds"`
	Baseline      int64 `json:"baseline"`
}

type syncState struct {
	// DeviceID identifies this installation to the account-sync backend only.
	// It must never come from or be compared with clientid's installation id:
	// that id is deliberately pseudonymous and unlinked from any account, and
	// reusing it here would deanonymize telemetry collected under it.
	DeviceID         string               `json:"deviceId"`
	SettingsRevision int64                `json:"settingsRevision"`
	Games            map[string]gameState `json:"games"`
}

func emptyState() syncState {
	return syncState{Games: map[string]gameState{}}
}

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) path() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, "sync.json")
}

func (s *store) load() (syncState, error) {
	path := s.path()
	if path == "" {
		return syncState{}, errors.New("accountsync state path unavailable")
	}
	var st syncState
	err := storage.Load(path, stateVersion, nil, &st)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return syncState{}, fmt.Errorf("load accountsync state: %w", err)
	}
	if st.Games == nil {
		st.Games = map[string]gameState{}
	}
	return st, nil
}

func (s *store) save(st syncState) error {
	path := s.path()
	if path == "" {
		return errors.New("accountsync state path unavailable")
	}
	return storage.Save(path, stateVersion, st)
}
