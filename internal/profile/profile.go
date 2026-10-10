package profile

import (
	"errors"
	"fmt"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/playlog"
)

type Library interface {
	GetGames() []library.Game
	GetRunningGames() []string
}

type Log interface {
	Since(t time.Time) []playlog.Session
}

type Catalog interface {
	GetGames(ids []string) []catalog.Game
	GameByIGDB(igdbID string) (catalog.Game, bool)
	IGDBIDOf(id string) string
}

type Service struct {
	library  Library
	log      Log
	catalog  Catalog
	showcase func() []string
	layout   func() []account.LayoutBlock
	now      func() time.Time
}

//wails:ignore
func NewService(lib Library, log Log, cat Catalog, showcase func() []string, layout func() []account.LayoutBlock) (*Service, error) {
	switch {
	case lib == nil:
		return nil, errors.New("profile: library is required")
	case log == nil:
		return nil, errors.New("profile: play log is required")
	case cat == nil:
		return nil, errors.New("profile: catalog is required")
	case showcase == nil:
		return nil, errors.New("profile: showcase source is required")
	case layout == nil:
		return nil, errors.New("profile: layout source is required")
	}
	return &Service{library: lib, log: log, catalog: cat, showcase: showcase, layout: layout, now: time.Now}, nil
}

func (s *Service) Snapshot() (Snapshot, error) {
	return s.snapshot(s.showcase())
}

// Preview includes disabled showcases without changing the saved profile.
func (s *Service) Preview() (Snapshot, error) {
	return s.snapshot([]string{"favorites", "recently_completed", "most_played"})
}

func (s *Service) snapshot(showcase []string) (Snapshot, error) {
	now := s.now()
	monthStart := MonthStart(now)
	since := minTime(monthStart, now.Add(-recentWindow))
	games := s.library.GetGames()
	if history, ok := s.library.(interface{ GetHistoryGames() []library.Game }); ok {
		games = history.GetHistoryGames()
	}
	snap := Build(games, s.log.Since(since), s.library.GetRunningGames(), showcase, now)
	snap.Genres = ComputeGenres(s.playedGenres(games))

	layoutGames, err := s.layoutGames(s.layout(), games)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve layout games: %w", err)
	}
	snap.LayoutGames = layoutGames
	return snap, nil
}

func (s *Service) playedGenres(games []library.Game) []PlayedGenres {
	genresOf := map[string][]string{}
	played := make([]PlayedGenres, 0, len(games))
	for _, g := range games {
		if g.PlaytimeSeconds <= 0 {
			continue
		}
		entry := PlayedGenres{Seconds: g.PlaytimeSeconds}
		if g.CanonicalGameID != "" {
			genres, known := genresOf[g.CanonicalGameID]
			if !known {
				if found := s.catalog.GetGames([]string{g.CanonicalGameID}); len(found) == 1 {
					genres = found[0].Genres
				}
				genresOf[g.CanonicalGameID] = genres
			}
			entry.Genres = genres
		}
		played = append(played, entry)
	}
	return played
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
