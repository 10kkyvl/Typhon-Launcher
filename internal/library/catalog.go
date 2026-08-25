package library

import (
	"fmt"
	"log/slog"
	"strings"
)

func (s *Service) AddCatalogGame(canonicalGameID, title, cover string) (Game, error) {
	canonicalGameID = strings.TrimSpace(canonicalGameID)
	if canonicalGameID == "" {
		return Game{}, errEmptyCanonicalGameID
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Game{}, errEmptyCatalogTitle
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].CanonicalGameID == canonicalGameID {
			return s.games[i], nil
		}
	}

	game := Game{
		ID:              newID(),
		Title:           title,
		Cover:           cover,
		CanonicalGameID: canonicalGameID,
		Source:          SourceManaged,
		Uninstalled:     true,
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("catalog game added", "id", game.ID, "title", game.Title)
	s.emitUpdated()
	return game, nil
}
