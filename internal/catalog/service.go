package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/titles"
)

const (
	gamesVersion     = 1
	overridesVersion = 1
	maxAliasLen      = 120
	defaultSearchCap = 20
)

var errNotFound = errors.New("игра не найдена")

type Service struct {
	mu            sync.Mutex
	gamesPath     string
	overridesPath string
	games         []Game
	overrides     []MatchOverride
	overrideMap   map[string]string
	idx           *index
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(dir)
}

//wails:ignore
func NewServiceAt(dir string) (*Service, error) {
	if dir == "" {
		return nil, errors.New("catalog dir unavailable")
	}
	s := &Service{overrideMap: map[string]string{}}
	s.gamesPath = filepath.Join(dir, "catalog.json")
	s.overridesPath = filepath.Join(dir, "match_overrides.json")
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) load() error {
	if err := loadList(s.gamesPath, gamesVersion, &s.games); err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	if err := loadList(s.overridesPath, overridesVersion, &s.overrides); err != nil {
		return fmt.Errorf("load match overrides: %w", err)
	}
	s.rebuildLocked()
	return nil
}

func loadList(path string, version int, out any) error {
	err := storage.Load(path, version, nil, out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) rebuildLocked() {
	s.idx = buildIndex(s.games)
	s.overrideMap = make(map[string]string, len(s.overrides))
	for _, o := range s.overrides {
		if o.Pattern == "" || o.GameID == "" {
			continue
		}
		s.overrideMap[o.Pattern] = o.GameID
	}
}

func (s *Service) persistGamesLocked() error {
	if s.gamesPath == "" {
		return errors.New("catalog path unavailable")
	}
	return storage.Save(s.gamesPath, gamesVersion, s.games)
}

func (s *Service) persistOverridesLocked() error {
	if s.overridesPath == "" {
		return errors.New("match overrides path unavailable")
	}
	return storage.Save(s.overridesPath, overridesVersion, s.overrides)
}

func (s *Service) ListGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Game(nil), s.games...)
}

func (s *Service) GetGame(id string) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game, ok := s.idx.game(id)
	if !ok {
		return Game{}, errNotFound
	}
	return game, nil
}

func (s *Service) SearchGames(query string, limit int) []Game {
	if limit <= 0 {
		limit = defaultSearchCap
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx.search(query, limit)
}

func (s *Service) ListOverrides() []MatchOverride {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]MatchOverride(nil), s.overrides...)
}

//wails:ignore
func (s *Service) Resolve(q Query) Match {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx.resolve(normalizeQuery(q), s.overrideMap)
}

//wails:ignore
func (s *Service) ResolveAll(queries []Query) []Match {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Match, len(queries))
	for i, q := range queries {
		out[i] = s.idx.resolve(normalizeQuery(q), s.overrideMap)
	}
	return out
}

//wails:ignore
func (s *Service) Provision(queries []Query) map[string]Game {
	s.mu.Lock()
	defer s.mu.Unlock()

	created := 0
	out := make(map[string]Game, len(queries))
	for _, q := range queries {
		q = normalizeQuery(q)
		if q.Normalized == "" {
			continue
		}
		if _, ok := out[q.Normalized]; ok {
			continue
		}
		if positions := s.idx.byTitle[q.Normalized]; len(positions) > 0 {
			out[q.Normalized] = s.idx.entries[positions[0]].game
			continue
		}
		game := newGame(q)
		s.games = append(s.games, game)
		s.idx.add(game)
		out[q.Normalized] = game
		created++
	}
	if created > 0 {
		if err := s.persistGamesLocked(); err != nil {
			slog.Error("save catalog", "error", err)
		}
		slog.Info("catalog games provisioned", "created", created, "total", len(s.games))
	}
	return out
}

func (s *Service) AddGame(game Game) (Game, error) {
	game.Title = strings.TrimSpace(game.Title)
	if game.Title == "" {
		return Game{}, errors.New("укажите название игры")
	}
	if game.ID == "" {
		game.ID = NewID()
	}
	if game.SortTitle == "" {
		game.SortTitle = sortTitle(game.Title)
	}
	if game.CreatedAt.IsZero() {
		game.CreatedAt = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.idx.byID[game.ID]; exists {
		return Game{}, errors.New("игра с таким идентификатором уже есть")
	}
	s.games = append(s.games, game)
	s.idx.add(game)
	if err := s.persistGamesLocked(); err != nil {
		return Game{}, err
	}
	return game, nil
}

//wails:ignore
func (s *Service) EnsureGame(title string, year int) (Game, error) {
	games := s.Provision([]Query{{Title: title, Year: year}})
	normalized := titles.Normalize(title)
	game, ok := games[normalized]
	if !ok {
		return Game{}, errNotFound
	}
	return game, nil
}

//wails:ignore
func (s *Service) LearnMatch(normalized, gameID string) error {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" || gameID == "" {
		return errors.New("нечего запоминать")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	pos, ok := s.idx.byID[gameID]
	if !ok {
		return errNotFound
	}
	game := s.idx.entries[pos].game

	replaced := false
	for i := range s.overrides {
		if s.overrides[i].Pattern == normalized {
			s.overrides[i].GameID = gameID
			s.overrides[i].CreatedAt = time.Now()
			replaced = true
			break
		}
	}
	if !replaced {
		s.overrides = append(s.overrides, MatchOverride{Pattern: normalized, GameID: gameID, CreatedAt: time.Now()})
	}
	if err := s.persistOverridesLocked(); err != nil {
		return fmt.Errorf("save overrides: %w", err)
	}

	if learnable(normalized, game) {
		for i := range s.games {
			if s.games[i].ID != gameID {
				continue
			}
			s.games[i].Aliases = append(s.games[i].Aliases, normalized)
			if err := s.persistGamesLocked(); err != nil {
				slog.Error("save catalog", "error", err)
			}
			break
		}
	}
	s.rebuildLocked()
	slog.Info("match override saved", "pattern", normalized, "game", gameID)
	return nil
}

//wails:ignore
func (s *Service) ForgetMatch(normalized string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.overrides {
		if s.overrides[i].Pattern != normalized {
			continue
		}
		s.overrides = append(s.overrides[:i], s.overrides[i+1:]...)
		if err := s.persistOverridesLocked(); err != nil {
			return err
		}
		s.rebuildLocked()
		return nil
	}
	return nil
}

func learnable(normalized string, game Game) bool {
	if len(normalized) > maxAliasLen || len(normalized) < 2 {
		return false
	}
	if normalized == titles.Normalize(game.Title) {
		return false
	}
	for _, alias := range game.Aliases {
		if titles.Normalize(alias) == normalized {
			return false
		}
	}
	return true
}

func normalizeQuery(q Query) Query {
	q.Title = strings.TrimSpace(q.Title)
	if q.Normalized == "" {
		q.Normalized = titles.Normalize(q.Title)
	} else {
		q.Normalized = titles.Normalize(q.Normalized)
	}
	return q
}

func newGame(q Query) Game {
	title := q.Title
	if title == "" {
		title = q.Normalized
	}
	game := Game{
		ID:          NewID(),
		Title:       title,
		SortTitle:   sortTitle(title),
		Developer:   q.Developer,
		ExternalIDs: q.ExternalIDs,
		Provisional: true,
		CreatedAt:   time.Now(),
	}
	if q.Year > 0 {
		year := q.Year
		game.ReleaseYear = &year
	}
	return game
}

func sortTitle(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			return strings.TrimSpace(lower[len(article):])
		}
	}
	return lower
}

//wails:ignore
func NewID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("c%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

//wails:ignore
func (s *Service) TitleOf(id string) string {
	if id == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	game, ok := s.idx.game(id)
	if !ok {
		return ""
	}
	return game.Title
}

//wails:ignore
func (s *Service) LookupByTitle(title string) (Game, bool) {
	normalized := titles.Normalize(title)
	if normalized == "" {
		return Game{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if gameID, ok := s.overrideMap[normalized]; ok {
		if game, ok := s.idx.game(gameID); ok {
			return game, true
		}
	}
	positions := s.idx.byTitle[normalized]
	if len(positions) == 0 {
		positions = s.idx.byAlias[normalized]
	}
	if len(positions) == 0 {
		return Game{}, false
	}
	return s.idx.entries[positions[0]].game, true
}
