// Package compat ведёт журнал того, как игры на самом деле запускаются на этой
// машине. Список нерабочих игр набирается сам: пользователь не обязан вести
// его руками, а без него он каждый раз первооткрыватель.
package compat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

// State — что журнал думает об игре.
type State string

const (
	// StateUnknown: данных не хватает. Одна неудача сюда же — игра могла
	// упасть от чего угодно, и клеймить её с первого раза значит врать.
	StateUnknown State = "unknown"
	// StateWorks: игра доказала, что запускается.
	StateWorks State = "works"
	// StateBroken: подряд не запускается.
	StateBroken State = "broken"
)

const (
	// playedSeconds — с какой длительности считаем, что игра точно
	// запустилась. Минуты хватает: до меню доходят и медленные сборки.
	playedSeconds = 60
	// selfExitSeconds — короче этого закрывшаяся сама игра считается упавшей.
	// Пользовательская остановка сюда не относится: он мог просто передумать.
	selfExitSeconds = 30
	// brokenAfter — сколько неудач подряд превращают «не знаю» в «не работает».
	brokenAfter = 2
)

// Record — что известно про одну игру.
type Record struct {
	GameID      string    `json:"gameId"`
	Attempts    int       `json:"attempts"`
	Failures    int       `json:"failures"`
	BestSeconds int64     `json:"bestSeconds"`
	LastError   string    `json:"lastError,omitempty"`
	LastAt      time.Time `json:"lastAt"`
}

// Status — ответ на вопрос «работает ли эта игра», в том виде, в каком его
// показывают пользователю.
type Status struct {
	GameID      string `json:"gameId"`
	State       State  `json:"state"`
	Attempts    int    `json:"attempts"`
	Failures    int    `json:"failures"`
	BestSeconds int64  `json:"bestSeconds"`
	LastError   string `json:"lastError,omitempty"`
}

type Service struct {
	path string

	mu      sync.Mutex
	records map[string]*Record
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("compat: каталог конфигурации: %w", err)
	}
	return NewServiceAt(filepath.Join(dir, "compat.json"))
}

func NewServiceAt(path string) (*Service, error) {
	s := &Service{path: path, records: map[string]*Record{}}
	s.load()
	return s, nil
}

// load не возвращает ошибку намеренно: совместимость — подсказка, а не
// состояние, без которого лаунчер не работает. Битый журнал начинается заново.
func (s *Service) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("read compat journal", "path", s.path, "error", err)
		}
		return
	}
	var list []Record
	if err := json.Unmarshal(raw, &list); err != nil {
		slog.Warn("parse compat journal", "path", s.path, "error", err)
		return
	}
	for i := range list {
		if list[i].GameID == "" {
			continue
		}
		record := list[i]
		s.records[record.GameID] = &record
	}
}

func (s *Service) persistLocked() {
	list := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		list = append(list, *r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].GameID < list[j].GameID })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		slog.Warn("encode compat journal", "error", err)
		return
	}
	if err := storage.WriteAtomic(s.path, append(data, '\n')); err != nil {
		slog.Warn("write compat journal", "path", s.path, "error", err)
	}
}

// RecordLaunchFailure отмечает, что игра не запустилась вовсе.
//
//wails:ignore
func (s *Service) RecordLaunchFailure(gameID, reason string) {
	if gameID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recordLocked(gameID)
	r.Attempts++
	r.Failures++
	r.LastError = reason
	r.LastAt = time.Now().UTC()
	s.persistLocked()
}

// RecordSession отмечает завершившуюся сессию. stoppedByUser отделяет «игра
// умерла сама» от «её закрыли»: второе про совместимость не говорит ничего.
//
//wails:ignore
func (s *Service) RecordSession(gameID string, played time.Duration, stoppedByUser bool) {
	if gameID == "" {
		return
	}
	seconds := int64(played.Seconds())
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recordLocked(gameID)
	r.Attempts++
	r.LastAt = time.Now().UTC()
	if seconds > r.BestSeconds {
		r.BestSeconds = seconds
	}
	switch {
	case seconds >= playedSeconds:
		// Игра доказала, что работает: прошлые неудачи больше ничего не значат.
		r.Failures = 0
		r.LastError = ""
	case !stoppedByUser && seconds < selfExitSeconds:
		r.Failures++
		r.LastError = "игра закрылась сама сразу после запуска"
	}
	s.persistLocked()
}

func (s *Service) recordLocked(gameID string) *Record {
	if r, ok := s.records[gameID]; ok {
		return r
	}
	r := &Record{GameID: gameID}
	s.records[gameID] = r
	return r
}

func (s *Service) Status(gameID string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[gameID]
	if !ok {
		return Status{GameID: gameID, State: StateUnknown}
	}
	return statusOf(*r)
}

// Broken перечисляет игры, которые подряд не запускаются, — тот самый список,
// который иначе пришлось бы вести руками.
func (s *Service) Broken() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Status, 0, len(s.records))
	for _, r := range s.records {
		if st := statusOf(*r); st.State == StateBroken {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GameID < out[j].GameID })
	return out
}

// All отдаёт весь журнал: интерфейсу нужен разом весь список, а не запрос на
// каждую карточку.
func (s *Service) All() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Status, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, statusOf(*r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GameID < out[j].GameID })
	return out
}

func statusOf(r Record) Status {
	st := Status{
		GameID:      r.GameID,
		State:       StateUnknown,
		Attempts:    r.Attempts,
		Failures:    r.Failures,
		BestSeconds: r.BestSeconds,
		LastError:   r.LastError,
	}
	switch {
	case r.BestSeconds >= playedSeconds:
		st.State = StateWorks
	case r.Failures >= brokenAfter:
		st.State = StateBroken
	}
	return st
}
