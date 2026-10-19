package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"typhon/internal/catalog"
	"typhon/internal/platform"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	savesVersion  = 1
	savesFileName = "save_paths.json"

	savesTTL        = 30 * 24 * time.Hour
	savesMissingTTL = 7 * 24 * time.Hour
	savesTimeout    = 30 * time.Second
	savesFailureTTL = 5 * time.Minute

	maxSavePaths   = 20
	maxSavePathLen = 260
	maxSaveAttrib  = 100

	pcgwHost = "www.pcgamingwiki.com"
)

var (
	ErrNoSaveData      = uierr.New("metadata.no_save_data", "для этой игры нет данных о путях сохранений")
	ErrSaveDataInvalid = uierr.New("metadata.save_data_invalid", "сервер вернул некорректные данные о сохранениях")
	ErrGameNotResolved = uierr.New("metadata.game_not_resolved", "игра ещё не сопоставлена с метаданными")
)

// SavePaths — пути сохранений игры по данным PCGamingWiki. Пути записаны
// шаблонами с токенами ({appdata}, {game}, ...), раскрывает их библиотека.
type SavePaths struct {
	PageURL string   `json:"pageUrl"`
	Windows []string `json:"windows"`
	Source  string   `json:"source"`
	License string   `json:"license"`
	Via     string   `json:"via,omitempty"`
}

// SaveProvider — необязательная возможность провайдера: старый или чужой
// провайдер без неё просто не даёт подсказок о сохранениях.
//
//wails:ignore
type SaveProvider interface {
	SavePaths(ctx context.Context, providerID string) (SavePaths, error)
}

func ValidSaveHint(hint string) bool {
	if utf8.RuneCountInString(hint) > maxSavePathLen {
		return false
	}
	_, _, err := platform.SplitSaveHint(hint)
	return err == nil
}

func validPageURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != pcgwHost || u.User != nil ||
		u.Opaque != "" || !strings.HasPrefix(u.Path, "/wiki/") || len(u.Path) <= len("/wiki/") {
		return "", false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return u.String(), true
}

// ValidateSavePaths проверяет ответ сервера на границе доверия. Страница не с
// PCGamingWiki отвергает ответ целиком; негодный путь выбрасывается по
// одному, чтобы одна кривая запись не лишала игру остальных подсказок.
func ValidateSavePaths(in SavePaths) (SavePaths, error) {
	page, ok := validPageURL(in.PageURL)
	if !ok {
		return SavePaths{}, fmt.Errorf("%w: адрес страницы %q", ErrSaveDataInvalid, in.PageURL)
	}
	out := SavePaths{
		PageURL: page,
		Windows: []string{},
		Source:  cleanText(in.Source, maxSaveAttrib),
		License: cleanText(in.License, maxSaveAttrib),
		Via:     cleanText(in.Via, maxSaveAttrib),
	}
	for _, hint := range in.Windows {
		if len(out.Windows) == maxSavePaths {
			slog.Warn("save paths over the limit dropped", "limit", maxSavePaths)
			break
		}
		if !ValidSaveHint(hint) {
			slog.Warn("invalid save path dropped", "path", hint)
			continue
		}
		out.Windows = append(out.Windows, hint)
	}
	return out, nil
}

type savesEntry struct {
	ProviderID string     `json:"providerId"`
	FetchedAt  time.Time  `json:"fetchedAt"`
	Missing    bool       `json:"missing,omitempty"`
	Paths      *SavePaths `json:"paths,omitempty"`
}

type savesFlight struct {
	done    chan struct{}
	res     SavePaths
	err     error
	waiters int
}

type savesFailure struct {
	err   error
	until time.Time
}

type savesCache struct {
	mu       sync.Mutex
	path     string
	entries  map[string]savesEntry
	flights  map[string]*savesFlight
	failures map[string]savesFailure
}

// join делает вызывающего ведущим, если по ключу ничего не летит, и
// присоединяет к летящему запросу иначе.
func (c *savesCache) join(key string) (*savesFlight, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.flights[key]; ok {
		f.waiters++
		return f, false
	}
	f := &savesFlight{done: make(chan struct{})}
	c.flights[key] = f
	return f, true
}

func (c *savesCache) finish(key string, f *savesFlight, res SavePaths, err error) {
	c.mu.Lock()
	f.res, f.err = res, err
	delete(c.flights, key)
	c.mu.Unlock()
	close(f.done)
}

func (c *savesCache) waiting(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.flights[key]; ok {
		return f.waiters
	}
	return 0
}

func (c *savesCache) noteFailure(key string, err error, until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures[key] = savesFailure{err: err, until: until}
}

func (c *savesCache) recentFailure(key string, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, ok := c.failures[key]
	if !ok {
		return nil
	}
	if !now.Before(f.until) {
		delete(c.failures, key)
		return nil
	}
	return f.err
}

func newSavesCache(dir string) (*savesCache, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errStorePath
	}
	c := &savesCache{
		path:     filepath.Join(dir, savesFileName),
		entries:  map[string]savesEntry{},
		flights:  map[string]*savesFlight{},
		failures: map[string]savesFailure{},
	}
	var stored []savesEntry
	err := storage.Load(c.path, savesVersion, nil, &stored)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load save paths: %w", err)
	}
	for _, e := range stored {
		if e.ProviderID != "" {
			c.entries[e.ProviderID] = e
		}
	}
	return c, nil
}

func (c *savesCache) lookup(providerID string, now time.Time) (savesEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[providerID]
	if !ok {
		return savesEntry{}, false
	}
	ttl := savesTTL
	if e.Missing {
		ttl = savesMissingTTL
	}
	if now.Sub(e.FetchedAt) > ttl {
		return savesEntry{}, false
	}
	return e, true
}

func (c *savesCache) peek(providerID string) (savesEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[providerID]
	return e, ok
}

func (c *savesCache) put(e savesEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, had := c.entries[e.ProviderID]
	c.entries[e.ProviderID] = e
	list := make([]savesEntry, 0, len(c.entries))
	for _, v := range c.entries {
		list = append(list, v)
	}
	if err := storage.Save(c.path, savesVersion, list); err != nil {
		if had {
			c.entries[e.ProviderID] = prev
		} else {
			delete(c.entries, e.ProviderID)
		}
		return fmt.Errorf("save save paths: %w", err)
	}
	return nil
}

func providerKey(game catalog.Game) string {
	if game.ExternalIDs.IGDB != "" {
		return game.ExternalIDs.IGDB
	}
	if game.ExternalIDs.Steam != "" {
		return "steam:" + game.ExternalIDs.Steam
	}
	return ""
}

// SavePaths отдаёт пути сохранений игры: из кэша, пока он свеж, иначе с сервера.
// ErrNoSaveData значит, что у игры нет страницы PCGamingWiki; это отличимо от
// сетевой ошибки, и отсутствие страницы тоже кэшируется, но ненадолго.
// Одновременные вызовы по одной игре сливаются в один запрос, а временный
// сбой на несколько минут запоминается в памяти: поиск сохранений не должен
// ходить в сеть при каждом открытии карточки.
func (s *Service) SavePaths(ctx context.Context, gameID string) (SavePaths, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return SavePaths{}, errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return SavePaths{}, err
	}
	key := providerKey(game)
	if key == "" {
		return SavePaths{}, ErrGameNotResolved
	}
	provider, hasProvider := s.provider.(SaveProvider)

	for {
		if e, ok := s.saves.lookup(key, s.clock()); ok {
			if e.Missing || e.Paths == nil {
				return SavePaths{}, ErrNoSaveData
			}
			return *e.Paths, nil
		}
		if !hasProvider {
			return SavePaths{}, ErrNotConfigured
		}
		if failure := s.saves.recentFailure(key, s.clock()); failure != nil {
			return SavePaths{}, failure
		}
		flight, leader := s.saves.join(key)
		if leader {
			res, err := s.fetchSavePaths(ctx, provider, key)
			s.saves.finish(key, flight, res, err)
			return res, err
		}
		select {
		case <-flight.done:
		case <-ctx.Done():
			return SavePaths{}, ctx.Err()
		}
		if ctx.Err() == nil && (errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded)) {
			continue
		}
		return flight.res, flight.err
	}
}

func (s *Service) fetchSavePaths(ctx context.Context, provider SaveProvider, key string) (SavePaths, error) {
	callCtx, cancel := context.WithTimeout(requestLanguage(ctx, s.currentLanguage()), savesTimeout)
	defer cancel()

	if err := s.budget.acquire(callCtx, classUser); err != nil {
		return SavePaths{}, err
	}
	fetched, fetchErr := provider.SavePaths(callCtx, key)
	s.budget.release(classUser)
	if fetchErr != nil && !errors.Is(fetchErr, ErrNoSaveData) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SavePaths{}, ctxErr
		}
		err := s.notePenalty(fetchErr)
		s.saves.noteFailure(key, err, s.clock().Add(savesFailureTTL))
		return SavePaths{}, err
	}

	entry := savesEntry{ProviderID: key, FetchedAt: s.clock()}
	if fetchErr != nil {
		entry.Missing = true
	} else {
		valid, err := ValidateSavePaths(fetched)
		if err != nil {
			s.saves.noteFailure(key, err, s.clock().Add(savesFailureTTL))
			return SavePaths{}, err
		}
		entry.Paths = &valid
	}
	if err := s.saves.put(entry); err != nil {
		return SavePaths{}, uierr.Wrap("metadata.save_failed", err)
	}
	if entry.Missing {
		return SavePaths{}, ErrNoSaveData
	}
	return *entry.Paths, nil
}
