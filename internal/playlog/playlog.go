package playlog

import (
	"encoding/json"
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
	playlogVersion = 2
	Retention      = 90 * 24 * time.Hour
	daysRetention  = 400
	eventRecorded  = "playlog:recorded"
	eventDegraded  = "playlog:degraded"
	dayLayout      = "2006-01-02"
)

type DayEntry struct {
	Date     string `json:"date"`
	GameID   string `json:"gameId"`
	Seconds  int64  `json:"seconds"`
	Longest  int64  `json:"longest"`
	Sessions int    `json:"sessions"`
}

type document struct {
	Sessions []Session  `json:"sessions"`
	Days     []DayEntry `json:"days"`
}

type Session struct {
	GameID    string    `json:"gameId"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type DegradedStatus struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

type Service struct {
	mu       sync.Mutex
	path     string
	sessions []Session
	days     []DayEntry
	now      func() time.Time
	status   DegradedStatus
	titleOf  func(gameIDs []string) map[string]string
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

//wails:ignore
func NewServiceAt(path string) (*Service, error) {
	return newServiceAt(path, time.Now)
}

func newServiceAt(path string, now func() time.Time) (*Service, error) {
	if path == "" {
		return nil, errors.New("playlog path unavailable")
	}
	s := &Service{path: path, now: now}
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	s.sessions = prune(doc.Sessions, s.now())
	s.days = pruneDays(doc.Days, s.now())
	return s, nil
}

//wails:ignore
func (s *Service) SetTitleResolver(titleOf func(gameIDs []string) map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.titleOf = titleOf
}

func (s *Service) load() (document, error) {
	var doc document
	loc := s.now().Location()
	migrations := map[int]storage.Migration{1: func(raw json.RawMessage) (json.RawMessage, error) {
		return migrateV1(raw, loc)
	}}
	err := storage.Load(s.path, playlogVersion, migrations, &doc)
	if errors.Is(err, fs.ErrNotExist) {
		return document{}, nil
	}
	if err != nil {
		return document{}, fmt.Errorf("load playlog %s: %w", s.path, err)
	}
	return doc, nil
}

func migrateV1(raw json.RawMessage, loc *time.Location) (json.RawMessage, error) {
	var sessions []Session
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &sessions); err != nil {
			return nil, fmt.Errorf("parse v1 sessions: %w", err)
		}
	}
	var days []DayEntry
	for _, session := range sessions {
		days = addToDays(days, session, loc)
	}
	return json.Marshal(document{Sessions: sessions, Days: days})
}

func addToDays(days []DayEntry, session Session, loc *time.Location) []DayEntry {
	start := session.StartedAt.In(loc)
	end := session.EndedAt.In(loc)
	total := int64(end.Sub(start) / time.Second)
	cursor := start
	first := true
	for cursor.Before(end) {
		y, m, d := cursor.Date()
		next := time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		segmentEnd := end
		if next.Before(end) {
			segmentEnd = next
		}
		seconds := int64(segmentEnd.Sub(cursor) / time.Second)
		days = addDay(days, cursor.Format(dayLayout), session.GameID, seconds, total, first)
		first = false
		cursor = segmentEnd
	}
	return days
}

func addDay(days []DayEntry, date, gameID string, seconds, longest int64, countSession bool) []DayEntry {
	for i := range days {
		if days[i].Date != date || days[i].GameID != gameID {
			continue
		}
		days[i].Seconds += seconds
		if countSession {
			days[i].Sessions++
			if longest > days[i].Longest {
				days[i].Longest = longest
			}
		}
		return days
	}
	entry := DayEntry{Date: date, GameID: gameID, Seconds: seconds}
	if countSession {
		entry.Sessions = 1
		entry.Longest = longest
	}
	return append(days, entry)
}

func pruneDays(days []DayEntry, now time.Time) []DayEntry {
	cutoff := now.AddDate(0, 0, -daysRetention).Format(dayLayout)
	kept := days[:0]
	for _, day := range days {
		if day.Date >= cutoff {
			kept = append(kept, day)
		}
	}
	return kept
}

// Record appends a session, then persists the log. On a persist failure the
// in-memory sessions are rolled back to their state before the call, the
// service enters a degraded state (surfaced by playlog:degraded), and the
// failure is logged: SetPlayRecorder wires this method into
// library.Service's fixed, error-less callback signature, so there is no
// synchronous caller left to hand the error back to.
//
//wails:ignore
func (s *Service) Record(gameID string, startedAt, endedAt time.Time) {
	if gameID == "" || !endedAt.After(startedAt) {
		return
	}
	session := Session{GameID: gameID, StartedAt: startedAt, EndedAt: endedAt}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Session(nil), s.sessions...)
	previousDays := append([]DayEntry(nil), s.days...)
	now := s.now()
	s.sessions = prune(append(s.sessions, session), now)
	s.days = pruneDays(addToDays(append([]DayEntry(nil), s.days...), session, now.Location()), now)
	if err := storage.Save(s.path, playlogVersion, document{Sessions: s.sessions, Days: s.days}); err != nil {
		s.sessions = previous
		s.days = previousDays
		s.status = DegradedStatus{Degraded: true, Message: err.Error()}
		slog.Error("persist playlog", "path", s.path, "error", err)
		emit(eventDegraded, s.status)
		return
	}
	s.status = DegradedStatus{}
	emit(eventRecorded, session)
}

//wails:ignore
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
