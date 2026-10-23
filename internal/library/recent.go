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

// Running games come first, even never-played ones: otherwise the tray could
// not show or stop a game that fell out of the limit.
func (s *Service) RecentGames(limit int) []RecentGame {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		RecentGame
		lastPlayed time.Time
		played     bool
	}

	s.mu.Lock()
	candidates := make([]candidate, 0, len(s.games))
	for i := range s.games {
		game := &s.games[i]
		if game.Hidden || game.Uninstalled || game.Archived {
			continue
		}
		_, running := s.running[game.ID]
		_, starting := s.starting[game.ID]
		active := running || starting
		if game.LastPlayed == nil && !active {
			continue
		}
		c := candidate{RecentGame: RecentGame{
			ID:         game.ID,
			Title:      game.Title,
			Executable: game.Executable,
			Running:    active,
		}}
		if game.LastPlayed != nil {
			c.lastPlayed = *game.LastPlayed
			c.played = true
		}
		candidates = append(candidates, c)
	}
	s.mu.Unlock()

	slices.SortStableFunc(candidates, func(a, b candidate) int {
		if a.Running != b.Running {
			if a.Running {
				return -1
			}
			return 1
		}
		if a.played != b.played {
			if a.played {
				return -1
			}
			return 1
		}
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
