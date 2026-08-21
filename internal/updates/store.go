package updates

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

const (
	updatesVersion   = 1
	historyVersion   = 1
	rollbackVersion  = 1
	verifyVersion    = 1
	manifestsVersion = 1

	maxHistory = 200
)

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
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

func load[T any](path string, version int, label string) []T {
	if path == "" {
		return nil
	}
	var list []T
	if err := storage.Load(path, version, nil, &list); err != nil {
		slog.Error("load "+label, "error", err)
		return nil
	}
	return list
}

func (s *store) loadUpdates() []Update {
	return load[Update](s.path("updates.json"), updatesVersion, "updates")
}

func (s *store) saveUpdates(list []Update) error {
	return storage.Save(s.path("updates.json"), updatesVersion, list)
}

func (s *store) loadHistory() []UpdateHistory {
	return load[UpdateHistory](s.path("update_history.json"), historyVersion, "update history")
}

func (s *store) saveHistory(list []UpdateHistory) error {
	if len(list) > maxHistory {
		list = list[len(list)-maxHistory:]
	}
	return storage.Save(s.path("update_history.json"), historyVersion, list)
}

func (s *store) loadRollbacks() []Rollback {
	return load[Rollback](s.path("rollbacks.json"), rollbackVersion, "rollbacks")
}

func (s *store) saveRollbacks(list []Rollback) error {
	return storage.Save(s.path("rollbacks.json"), rollbackVersion, list)
}

func (s *store) loadVerifications() []VerifyState {
	return load[VerifyState](s.path("verify.json"), verifyVersion, "verifications")
}

func (s *store) saveVerifications(list []VerifyState) error {
	return storage.Save(s.path("verify.json"), verifyVersion, list)
}

func (s *store) loadManifest(gameID string) (FileManifest, bool) {
	path := s.manifestPath(gameID)
	if path == "" {
		return FileManifest{}, false
	}
	var manifest FileManifest
	if err := storage.Load(path, manifestsVersion, nil, &manifest); err != nil {
		slog.Error("load manifest", "game", gameID, "error", err)
		return FileManifest{}, false
	}
	if len(manifest.Entries) == 0 {
		return FileManifest{}, false
	}
	return manifest, true
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
