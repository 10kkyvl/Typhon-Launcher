package playlog

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	playlogVersion = 1
	Retention      = 90 * 24 * time.Hour
	eventRecorded  = "playlog:recorded"
	eventDegraded  = "playlog:degraded"
)

type Session struct {
	GameID    string    `json:"gameId"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type degradedStatus struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

type Service struct {
	mu       sync.Mutex
	path     string
	sessions []Session
	now      func() time.Time
	status   degradedStatus
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return nil, errors.New("config dir unavailable")
	}
	return NewServiceAt(filepath.Join(dir, "playlog.json"))
}

func NewServiceAt(path string) (*Service, error) {
	return newServiceAt(path, time.Now)
}

func newServiceAt(path string, now func() time.Time) (*Service, error) {
	if path == "" {
		return nil, errors.New("playlog path unavailable")
	}
	s := &Service{path: path, now: now}
	sessions, err := s.load()
	if err != nil {
		return nil, err
	}
	s.sessions = prune(sessions, s.now())
	return s, nil
}

func (s *Service) load() ([]Session, error) {
	var sessions []Session
	err := storage.Load(s.path, playlogVersion, nil, &sessions)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load playlog %s: %w", s.path, err)
	}
	return sessions, nil
}

// Record appends a session, then persists the log. On a persist failure the
// in-memory sessions are rolled back to their state before the call, the
// service enters a degraded state (surfaced by playlog:degraded), and the
// failure is logged: SetPlayRecorder wires this method into
// library.Service's fixed, error-less callback signature, so there is no
// synchronous caller left to hand the error back to.
func (s *Service) Record(gameID string, startedAt, endedAt time.Time) {
	if gameID == "" || !endedAt.After(startedAt) {
		return
	}
	session := Session{GameID: gameID, StartedAt: startedAt, EndedAt: endedAt}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Session(nil), s.sessions...)
	s.sessions = prune(append(s.sessions, session), s.now())
	if err := storage.Save(s.path, playlogVersion, s.sessions); err != nil {
		s.sessions = previous
		s.status = degradedStatus{Degraded: true, Message: err.Error()}
		slog.Error("persist playlog", "path", s.path, "error", err)
		emit(eventDegraded, s.status)
		return
	}
	s.status = degradedStatus{}
	emit(eventRecorded, session)
}

func (s *Service) Since(t time.Time) []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session.EndedAt.After(t) {
			out = append(out, session)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

func prune(sessions []Session, now time.Time) []Session {
	cutoff := now.Add(-Retention)
	kept := sessions[:0]
	for _, session := range sessions {
		if !session.EndedAt.Before(cutoff) {
			kept = append(kept, session)
		}
	}
	return kept
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}
