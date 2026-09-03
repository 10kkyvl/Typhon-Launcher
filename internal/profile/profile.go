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
	now := s.now()
	return Build(s.library.GetGames(), s.log.Since(now.Add(-recentWindow)), s.library.GetRunningGames(), s.showcase(), now)
}
