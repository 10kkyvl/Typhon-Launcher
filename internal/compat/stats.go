package compat

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	"typhon/internal/storage"
)

// AnyRepacker — строка агрегата, в которой сложены все сборщики. На карточке
// каталога релиз ещё не выбран, и цифра там нужна общая.
const AnyRepacker = "*"

// Shared — что общая статистика знает про одну игру или одну сборку.
type Shared struct {
	GameID   string `json:"gameId"`
	Repacker string `json:"repacker"`
	Works    int    `json:"works"`
	Total    int    `json:"total"`
}

// Ratio — доля машин, на которых игра запустилась. Total нулём не бывает:
// сервер не отдаёт строк, по которым нечего считать.
func (s Shared) Ratio() float64 {
	if s.Total <= 0 {
		return 0
	}
	return float64(s.Works) / float64(s.Total)
}

// Snapshot — весь агрегат разом. Он маленький: строки заводятся только на
// игры, про которые кто-то отчитался, и только выше порога наблюдений.
type Snapshot struct {
	ETag      string    `json:"etag,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	Games     []Shared  `json:"games"`
}

// Stats держит снимок агрегата и переживает перезапуск: без сети лаунчер
// показывает то, что знал в прошлый раз, а не пустой каталог.
type Stats struct {
	path string

	mu   sync.RWMutex
	etag string
	seen time.Time
	byID map[string]Shared
}

func NewStatsAt(path string) *Stats {
	s := &Stats{path: path, byID: map[string]Shared{}}
	s.load()
	return s
}

func statsKey(gameID, repacker string) string { return gameID + "\x00" + repacker }

// load молчит об ошибках намеренно: общая статистика — подсказка, а не
// состояние, без которого лаунчер не работает.
func (s *Stats) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("read compat stats", "path", s.path, "error", err)
		}
		return
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		slog.Warn("parse compat stats", "path", s.path, "error", err)
		return
	}
	s.replace(snapshot)
}

func (s *Stats) replace(snapshot Snapshot) {
	next := make(map[string]Shared, len(snapshot.Games))
	for _, g := range snapshot.Games {
		if g.GameID == "" || g.Total <= 0 {
			continue
		}
		next[statsKey(g.GameID, g.Repacker)] = g
	}
	s.mu.Lock()
	s.byID = next
	s.etag = snapshot.ETag
	s.seen = snapshot.UpdatedAt
	s.mu.Unlock()
}

func (s *Stats) Apply(snapshot Snapshot) {
	s.replace(snapshot)
	s.persist(snapshot)
}

func (s *Stats) persist(snapshot Snapshot) {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		slog.Warn("encode compat stats", "error", err)
		return
	}
	if err := storage.WriteAtomic(s.path, append(data, '\n')); err != nil {
		slog.Warn("write compat stats", "path", s.path, "error", err)
	}
}

func (s *Stats) ETag() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.etag
}

// Lookup отвечает про конкретную сборку, Game — про игру целиком. Второе
// значение ложно, когда наблюдений не набралось: молчание честнее, чем доля,
// посчитанная по одной машине.
func (s *Stats) Lookup(gameID, repacker string) (Shared, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	got, ok := s.byID[statsKey(gameID, repacker)]
	return got, ok
}

func (s *Stats) Game(gameID string) (Shared, bool) {
	return s.Lookup(gameID, AnyRepacker)
}

func (s *Stats) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID)
}
