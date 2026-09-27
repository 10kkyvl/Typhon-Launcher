package accountsync

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"typhon/internal/storage"
)

const stateVersion = 1

type gameState struct {
	DeviceSeconds int64 `json:"deviceSeconds"`
	Baseline      int64 `json:"baseline"`
}

type syncState struct {
	Owner      string               `json:"owner,omitempty"`
	Tombstones map[string]time.Time `json:"tombstones,omitempty"`
	// DeviceID identifies this installation to the account-sync backend only.
	// It must never come from or be compared with clientid's installation id:
	// that id is deliberately pseudonymous and unlinked from any account, and
	// reusing it here would deanonymize telemetry collected under it.
	DeviceID         string               `json:"deviceId"`
	SettingsRevision int64                `json:"settingsRevision"`
	Games            map[string]gameState `json:"games"`
	// Removed holds this device's unconfirmed deletions, keyed by IGDB id.
	// An entry stays here from the moment the game disappears from the
	// local library until the server accepts a push carrying it, so an
	// offline removal survives a restart and is retried on the next sync.
	Removed map[string]time.Time `json:"removed"`
}

func emptyState() syncState {
	return syncState{Games: map[string]gameState{}, Removed: map[string]time.Time{}}
}

type store struct {
	dir   string
	owner string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) path() string {
	if s.dir == "" {
		return ""
	}
	if s.owner != "" {
		return filepath.Join(s.dir, fmt.Sprintf("sync-%x.json", sha256.Sum256([]byte(s.owner))))
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
	if st.Removed == nil {
		st.Removed = map[string]time.Time{}
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
