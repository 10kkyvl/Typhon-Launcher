package library

import (
	"fmt"
	"time"
)

type SyncGame struct {
	CanonicalGameID string
	PlaytimeSeconds int64
	LastPlayed      *time.Time
	Owned           bool
}

//wails:ignore
func (s *Service) SyncSnapshot() []SyncGame {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SyncGame, 0, len(s.games))
	for i := range s.games {
		if s.games[i].CanonicalGameID == "" {
			continue
		}
		item := SyncGame{
			CanonicalGameID: s.games[i].CanonicalGameID,
			PlaytimeSeconds: s.games[i].PlaytimeSeconds,
			Owned:           s.games[i].Owned,
		}
		if s.games[i].LastPlayed != nil {
			t := *s.games[i].LastPlayed
			item.LastPlayed = &t
		}
		out = append(out, item)
	}
	return out
}

//wails:ignore
func (s *Service) ApplySync(items []SyncGame) error {
	for _, item := range items {
		if item.CanonicalGameID == "" {
			return errEmptyCanonicalGameID
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous := append([]Game(nil), s.games...)
	changed := false
	for _, item := range items {
		for i := range s.games {
			if s.games[i].CanonicalGameID != item.CanonicalGameID {
				continue
			}
			if item.PlaytimeSeconds > s.games[i].PlaytimeSeconds {
				s.games[i].PlaytimeSeconds = item.PlaytimeSeconds
				changed = true
			}
			merged := laterOf(s.games[i].LastPlayed, item.LastPlayed)
			if !sameTime(s.games[i].LastPlayed, merged) {
				if merged != nil {
					t := *merged
					merged = &t
				}
				s.games[i].LastPlayed = merged
				changed = true
			}
			if item.Owned && !s.games[i].Owned {
				s.games[i].Owned = true
				changed = true
			}
			break
		}
	}

	if !changed {
		return nil
	}
	if err := s.persist(); err != nil {
		s.games = previous
		return fmt.Errorf("save library: %w", err)
	}
	return nil
}

func laterOf(local, incoming *time.Time) *time.Time {
	switch {
	case local == nil && incoming == nil:
		return nil
	case local == nil:
		return incoming
	case incoming == nil:
		return local
	case incoming.After(*local):
		return incoming
	default:
		return local
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
