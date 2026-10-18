package cloudsaves

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
	LocalDigest  string    `json:"localDigest"`
	RemoteDigest string    `json:"remoteDigest"`
	SyncedAt     time.Time `json:"syncedAt"`
	Conflict     *Conflict `json:"conflict,omitempty"`
	// Excluded marks a game whose cloud copy the player deleted: passes leave
	// it alone until the player syncs it again by hand.
	Excluded bool `json:"excluded,omitempty"`
}

// syncState is keyed by the catalog game id, the key the cloud uses, so the
// same game on another device meets the same record.
type syncState struct {
	DeviceID string               `json:"deviceId"`
	Games    map[string]gameState `json:"games"`
}

type store struct {
	dir   string
	write func(path string, version int, data any) error
}

func (s store) path(owner string) string {
	return filepath.Join(s.dir, fmt.Sprintf("cloud-saves-%x.json", sha256.Sum256([]byte(owner))))
}

func (s store) tmpDir() string {
	return filepath.Join(s.dir, "cloud-saves-tmp")
}

// load treats only a missing file as an empty state: any other failure means
// the recorded digests are unknown, and syncing on a guess could overwrite
// the only good copy of a save.
func (s store) load(owner string) (syncState, error) {
	var st syncState
	err := storage.Load(s.path(owner), stateVersion, nil, &st)
	if errors.Is(err, fs.ErrNotExist) {
		return syncState{Games: map[string]gameState{}}, nil
	}
	if err != nil {
		return syncState{}, fmt.Errorf("load cloud saves state: %w", err)
	}
	if st.Games == nil {
		st.Games = map[string]gameState{}
	}
	return st, nil
}

func (s store) save(owner string, st syncState) error {
	return s.write(s.path(owner), stateVersion, st)
}
