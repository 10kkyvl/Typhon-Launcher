package profile

import (
	"time"

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

type Service struct {
	library  Library
	log      Log
	showcase func() []string
	now      func() time.Time
}

//wails:ignore
func NewService(lib Library, log Log, showcase func() []string) *Service {
	return &Service{library: lib, log: log, showcase: showcase, now: time.Now}
}

func (s *Service) Snapshot() Snapshot {
	return s.snapshot(s.showcase())
}

// Preview includes disabled showcases without changing the saved profile.
func (s *Service) Preview() Snapshot {
	return s.snapshot([]string{"favorites", "recently_completed", "most_played"})
}

func (s *Service) snapshot(showcase []string) Snapshot {
	now := s.now()
	monthStart := MonthStart(now)
	since := minTime(monthStart, now.Add(-recentWindow))
	games := s.library.GetGames()
	if history, ok := s.library.(interface{ GetHistoryGames() []library.Game }); ok {
		games = history.GetHistoryGames()
	}
	return Build(games, s.log.Since(since), s.library.GetRunningGames(), showcase, now)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
