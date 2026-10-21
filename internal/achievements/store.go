package achievements

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"typhon/internal/storage"
)

const storeFile = "achievements.json"

type record struct {
	AppID          string               `json:"appId"`
	Unlocked       map[string]time.Time `json:"unlocked"`
	Found          bool                 `json:"found,omitempty"`
	Partial        bool                 `json:"partial,omitempty"`
	ScannedAt      time.Time            `json:"scannedAt"`
	SchemaCachedAt time.Time            `json:"schemaCachedAt"`
	SchemaLang     string               `json:"schemaLang,omitempty"`
	SchemaNone     bool                 `json:"schemaNone,omitempty"`
	Schema         []SchemaItem         `json:"schema"`
}

func (r record) clone() record {
	r.Unlocked = maps.Clone(r.Unlocked)
	r.Schema = slices.Clone(r.Schema)
	return r
}

type state struct {
	Games map[string]record `json:"games"`
}

// store is not safe for concurrent use: the service holds its mutex around
// every call.
type store struct {
	path  string
	write func(path string, version int, data any) error
	games map[string]record
	// broken is set when the file exists but cannot be read. The service then
	// refuses every write, so the unreadable file is never replaced by an
	// empty one.
	broken error
}

func openStore(dir string, write func(path string, version int, data any) error) *store {
	st := &store{path: filepath.Join(dir, storeFile), write: write, games: map[string]record{}}
	var loaded state
	err := storage.Load(st.path, storeVersion, nil, &loaded)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		st.broken = fmt.Errorf("%w: %w", ErrStoreBroken, err)
	case loaded.Games != nil:
		st.games = loaded.Games
	}
	return st
}

// commit replaces the record in memory and on disk as one operation: when the
// write fails the memory goes back to what the disk holds.
func (st *store) commit(gameID string, next record) error {
	if st.broken != nil {
		return st.broken
	}
	prev, had := st.games[gameID]
	st.games[gameID] = next
	if err := st.write(st.path, storeVersion, state{Games: st.games}); err != nil {
		if had {
			st.games[gameID] = prev
		} else {
			delete(st.games, gameID)
		}
		return fmt.Errorf("сохранение достижений: %w", err)
	}
	return nil
}
