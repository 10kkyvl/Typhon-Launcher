package history

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const historyVersion = 1

const (
	eventRecorded = "history:recorded"
	eventUpdated  = "history:updated"
	eventDegraded = "history:degraded"
)

type Service struct {
	mu      sync.Mutex
	path    string
	records []Record
	status  Status
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return nil, errors.New("config dir unavailable")
	}
	return NewServiceAt(filepath.Join(dir, "history.json"))
}

//wails:ignore
func NewServiceAt(path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("history path unavailable")
	}
	s := &Service{path: path}
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	s.records = trimRecords(records, time.Now())
	return s, nil
}

func (s *Service) load() ([]Record, error) {
	var records []Record
	err := storage.Load(s.path, historyVersion, nil, &records)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	return records, nil
}

func (s *Service) persistLocked() error {
	return storage.Save(s.path, historyVersion, s.records)
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// Record validates and appends r, then persists the journal. On a persist
// failure the in-memory slice is rolled back to its state before the call,
// the service enters a degraded state (surfaced by StatusOf and
// history:degraded), and the error is returned to the caller.
//
//wails:ignore
func (s *Service) Record(r Record) error {
	if !validKind(r.Kind) {
		return ErrBadKind
	}
	title := strings.TrimSpace(r.Title)
	if title == "" {
		return ErrEmptyTitle
	}
	r.Title = title
	if r.ID == "" {
		r.ID = newID()
	}
	if r.At.IsZero() {
		r.At = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Record(nil), s.records...)
	s.records = trimRecords(append(s.records, r), time.Now())
	if err := s.persistLocked(); err != nil {
		s.records = previous
		s.status = Status{Degraded: true, Message: err.Error()}
		emit(eventDegraded, s.status)
		return fmt.Errorf("save history: %w", err)
	}
	s.status = Status{}
	emit(eventRecorded, r)
	if len(s.records) != len(previous)+1 {
		emit(eventUpdated, append([]Record(nil), s.records...))
	}
	return nil
}

func (s *Service) List(f Filter) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.records))
	for i := len(s.records) - 1; i >= 0; i-- {
		if !matchesFilter(s.records[i], f) {
			continue
		}
		out = append(out, s.records[i])
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}

func (s *Service) Recent(n int) []Record {
	return s.List(Filter{Limit: n})
}

func (s *Service) StatusOf() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.records
	s.records = nil
	if err := s.persistLocked(); err != nil {
		s.records = previous
		s.status = Status{Degraded: true, Message: err.Error()}
		emit(eventDegraded, s.status)
		return fmt.Errorf("clear history: %w", err)
	}
	s.status = Status{}
	emit(eventUpdated, []Record{})
	return nil
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("h%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
