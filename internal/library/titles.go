package library

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

var errNoTitleResolver = errors.New("источник названий недоступен")

//wails:ignore
func (s *Service) SyncTitles(resolve func(canonicalGameID, releaseID string) string) error {
	if resolve == nil {
		return errNoTitleResolver
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := make([]Game, len(s.games))
	copy(previous, s.games)

	changed := make([]int, 0, len(s.games))
	for i := range s.games {
		if s.games[i].CanonicalGameID == "" && s.games[i].ReleaseID == "" {
			continue
		}
		title := strings.TrimSpace(resolve(s.games[i].CanonicalGameID, s.games[i].ReleaseID))
		if title == "" || title == s.games[i].Title {
			continue
		}
		s.games[i].Title = title
		changed = append(changed, i)
	}
	if len(changed) == 0 {
		return nil
	}
	if err := s.persist(); err != nil {
		s.games = previous
		return fmt.Errorf("save library: %w", err)
	}
	for _, i := range changed {
		slog.Info("game title synced", "id", s.games[i].ID, "title", s.games[i].Title, "was", previous[i].Title)
		if !s.games[i].Uninstalled {
			markInstalled(s.games[i])
		}
	}
	s.emitUpdated()
	return nil
}
