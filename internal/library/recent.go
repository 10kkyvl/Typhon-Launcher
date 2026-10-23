package library

import (
	"cmp"
	"slices"
	"time"
)

type RecentGame struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Executable string `json:"executable,omitempty"`
	Running    bool   `json:"running"`
}

// RecentGames — единственный источник «последних игр» для меню трея и Jump
// List: установленные, не скрытые, не заархивированные игры с известным
// LastPlayed, новые первыми. Под локом только копируются кандидаты.
func (s *Service) RecentGames(limit int) []RecentGame {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		RecentGame
		lastPlayed time.Time
	}

	s.mu.Lock()
	candidates := make([]candidate, 0, len(s.games))
	for i := range s.games {
		game := &s.games[i]
		if game.Hidden || game.Uninstalled || game.Archived || game.LastPlayed == nil {
			continue
		}
		_, running := s.running[game.ID]
		_, starting := s.starting[game.ID]
		candidates = append(candidates, candidate{
			RecentGame: RecentGame{
				ID:         game.ID,
				Title:      game.Title,
				Executable: game.Executable,
				Running:    running || starting,
			},
			lastPlayed: *game.LastPlayed,
		})
	}
	s.mu.Unlock()

	slices.SortStableFunc(candidates, func(a, b candidate) int {
		if c := b.lastPlayed.Compare(a.lastPlayed); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	games := make([]RecentGame, len(candidates))
	for i, c := range candidates {
		games[i] = c.RecentGame
	}
	return games
}
