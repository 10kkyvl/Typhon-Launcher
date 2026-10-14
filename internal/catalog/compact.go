package catalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"typhon/internal/storage"
)

const compactBackupSuffix = ".before-compact"

type CompactResult struct {
	Removed int
	Kept    int
	Backup  string
}

//wails:ignore
func (s *Service) Compact(ctx context.Context, referenced []string) (CompactResult, error) {
	if err := ctx.Err(); err != nil {
		return CompactResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.recommendationLoadErr != nil {
		return CompactResult{}, fmt.Errorf("recommendation preferences unavailable, not compacting: %w", s.recommendationLoadErr)
	}

	keep := s.keptIDsLocked(referenced)
	kept := make([]Game, 0, len(s.games))
	for _, g := range s.games {
		if keepGame(g, keep) {
			kept = append(kept, g)
		}
	}
	removed := len(s.games) - len(kept)
	if removed == 0 {
		return CompactResult{Kept: len(kept)}, nil
	}
	if s.gamesPath == "" {
		return CompactResult{}, errors.New("catalog path unavailable")
	}
	backup, err := s.backupGamesLocked()
	if err != nil {
		return CompactResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return CompactResult{}, err
	}

	previous := s.games
	s.games = kept
	s.rebuildLocked()
	if err := s.persistGamesLocked(); err != nil {
		s.games = previous
		s.rebuildLocked()
		return CompactResult{}, err
	}
	slog.Info("catalog compacted", "removed", removed, "kept", len(kept), "backup", backup)
	return CompactResult{Removed: removed, Kept: len(kept), Backup: backup}, nil
}

func (s *Service) keptIDsLocked(referenced []string) map[string]bool {
	keep := make(map[string]bool, len(referenced)+2*len(s.redirects)+len(s.overrides)+len(s.preferences.NotInterested))
	for _, id := range referenced {
		keep[id] = true
	}
	for from, to := range s.redirects {
		keep[from] = true
		keep[to] = true
	}
	for _, o := range s.overrides {
		keep[o.GameID] = true
	}
	for _, id := range s.preferences.NotInterested {
		keep[id] = true
	}
	return keep
}

func keepGame(g Game, keep map[string]bool) bool {
	if g.MetadataUpdatedAt != nil || keep[g.ID] || keep[g.ServerID] {
		return true
	}
	if g.ServerID == "" {
		return !g.Provisional || !g.ExternalIDs.empty()
	}
	return false
}

func (s *Service) backupGamesLocked() (string, error) {
	path := s.gamesPath + compactBackupSuffix
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("inspect catalog backup: %w", err)
	}
	raw, err := os.ReadFile(s.gamesPath)
	if err != nil {
		return "", fmt.Errorf("read catalog for backup: %w", err)
	}
	if err := storage.WriteAtomic(path, raw); err != nil {
		return "", fmt.Errorf("write catalog backup: %w", err)
	}
	return path, nil
}
