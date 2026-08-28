package metadata

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"typhon/internal/storage"
)

const (
	attemptsVersion  = 1
	attemptsFileName = "metadata_attempts.json"

	transientBase    = 5 * time.Minute
	transientMax     = 6 * time.Hour
	unmatchedBase    = 6 * time.Hour
	unmatchedMax     = 30 * 24 * time.Hour
	foregroundWindow = 15 * time.Minute
	maxBackoffShift  = 10
	maxAttempts      = 20000
	flushInterval    = 30 * time.Second
)

type attemptKind string

const (
	attemptUnmatched attemptKind = "unmatched"
	attemptTransient attemptKind = "transient"
)

type attempt struct {
	GameID    string      `json:"gameId"`
	Kind      attemptKind `json:"kind"`
	Failures  int         `json:"failures"`
	LastAt    time.Time   `json:"lastAt"`
	NextAt    time.Time   `json:"nextAt"`
	Dismissed bool        `json:"dismissed,omitempty"`
}

type attemptStore struct {
	mu      sync.Mutex
	path    string
	now     func() time.Time
	records map[string]attempt
	dirty   bool
}

func newAttemptStore(dir string, now func() time.Time) (*attemptStore, error) {
	if dir == "" {
		return nil, errStorePath
	}
	if now == nil {
		now = time.Now
	}
	s := &attemptStore{
		path:    filepath.Join(dir, attemptsFileName),
		now:     now,
		records: map[string]attempt{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *attemptStore) load() error {
	var stored []attempt
	err := storage.Load(s.path, attemptsVersion, nil, &stored)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load metadata attempts: %w", err)
	}
	now := s.now()
	for _, a := range stored {
		if a.GameID == "" || (!a.Dismissed && !a.NextAt.After(now)) {
			continue
		}
		s.records[a.GameID] = a
	}
	return nil
}

// due отвечает, пора ли снова спрашивать провайдера об этой игре. Открытая
// пользователем карточка ждёт не полный backoff, а короткое окно: один запрос
// на игру раз в четверть часа фону не мешает, а страница перестаёт быть пустой.
func (s *attemptStore) due(gameID string, foreground bool) bool {
	if gameID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[gameID]
	if !ok {
		return true
	}
	if rec.Dismissed {
		return false
	}
	now := s.now()
	if !now.Before(rec.NextAt) {
		return true
	}
	return foreground && !now.Before(rec.LastAt.Add(foregroundWindow))
}

func (s *attemptStore) fail(gameID string, kind attemptKind) {
	if gameID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	rec := s.records[gameID]
	if rec.Kind != kind {
		rec.Failures = 0
	}
	rec.GameID = gameID
	rec.Kind = kind
	rec.Failures++
	rec.LastAt = now
	rec.NextAt = now.Add(backoff(kind, rec.Failures))
	s.records[gameID] = rec
	s.dirty = true
	s.trimLocked()
}

// dismiss запоминает, что пользователь отказался от поиска для этой игры, и
// возвращает прежнюю запись: неудачная запись файла обязана вернуть память в
// исходное состояние через restore.
func (s *attemptStore) dismiss(gameID string) (attempt, bool) {
	if gameID == "" {
		return attempt{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.records[gameID]
	rec := prev
	rec.GameID = gameID
	if rec.Kind == "" {
		rec.Kind = attemptUnmatched
	}
	rec.Dismissed = true
	rec.LastAt = s.now()
	s.records[gameID] = rec
	s.dirty = true
	s.trimLocked()
	return prev, existed
}

func (s *attemptStore) restore(gameID string, prev attempt, existed bool) {
	if gameID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existed {
		s.records[gameID] = prev
	} else {
		delete(s.records, gameID)
	}
	s.dirty = true
}

func (s *attemptStore) resume(gameID string) {
	if gameID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[gameID]; !ok || !rec.Dismissed {
		return
	}
	delete(s.records, gameID)
	s.dirty = true
}

func (s *attemptStore) state(gameID string) (attempt, bool) {
	if gameID == "" {
		return attempt{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[gameID]
	return rec, ok
}

func (s *attemptStore) succeed(gameID string) {
	if gameID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[gameID]; !ok {
		return
	}
	delete(s.records, gameID)
	s.dirty = true
}

func (s *attemptStore) flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	now := s.now()
	out := make([]attempt, 0, len(s.records))
	for _, rec := range s.records {
		if !rec.Dismissed && !rec.NextAt.After(now) {
			continue
		}
		out = append(out, rec)
	}
	s.dirty = false
	s.mu.Unlock()

	sort.Slice(out, func(a, b int) bool { return out[a].GameID < out[b].GameID })
	if err := storage.Save(s.path, attemptsVersion, out); err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return fmt.Errorf("save metadata attempts: %w", err)
	}
	return nil
}

func (s *attemptStore) trimLocked() {
	if len(s.records) <= maxAttempts {
		return
	}
	now := s.now()
	for id, rec := range s.records {
		if !rec.Dismissed && !rec.NextAt.After(now) {
			delete(s.records, id)
		}
	}
	if len(s.records) <= maxAttempts {
		return
	}
	ids := make([]string, 0, len(s.records))
	for id := range s.records {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		ra, rb := s.records[ids[a]], s.records[ids[b]]
		if ra.Dismissed != rb.Dismissed {
			return !ra.Dismissed
		}
		return ra.NextAt.Before(rb.NextAt)
	})
	for _, id := range ids[:len(s.records)-maxAttempts] {
		delete(s.records, id)
	}
}

func (s *attemptStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func backoff(kind attemptKind, failures int) time.Duration {
	base, limit := transientBase, transientMax
	if kind == attemptUnmatched {
		base, limit = unmatchedBase, unmatchedMax
	}
	if failures < 1 {
		failures = 1
	}
	return min(base<<min(failures-1, maxBackoffShift), limit)
}
