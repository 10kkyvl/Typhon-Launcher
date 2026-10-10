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
	counted := make([]library.Game, 0, len(games))
	var ids []string
	seen := map[string]bool{}
	for _, g := range games {
		if g.Archived || g.PlaytimeSeconds <= 0 {
			continue
		}
		counted = append(counted, g)
		if g.CanonicalGameID != "" && !seen[g.CanonicalGameID] {
			seen[g.CanonicalGameID] = true
			ids = append(ids, g.CanonicalGameID)
		}
	}

	genresOf := s.genresByCanonicalID(ids)
	played := make([]PlayedGenres, 0, len(counted))
	for _, g := range counted {
		played = append(played, PlayedGenres{Seconds: g.PlaytimeSeconds, Genres: genresOf[g.CanonicalGameID]})
	}
	return played
}

// The batch lookup returns games under their resolved id and drops duplicates, so an id it cannot match
// back (an alias, or a game the catalog lacks) is asked for alone; a game the catalog lacks stays genreless.
func (s *Service) genresByCanonicalID(ids []string) map[string][]string {
	genresOf := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return genresOf
	}
	found := map[string][]string{}
	for _, g := range s.catalog.GetGames(ids) {
		found[g.ID] = g.Genres
	}
	for _, id := range ids {
		if genres, ok := found[id]; ok {
			genresOf[id] = genres
			continue
		}
		if single := s.catalog.GetGames([]string{id}); len(single) == 1 {
			genresOf[id] = single[0].Genres
		}
	}
	return genresOf
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
