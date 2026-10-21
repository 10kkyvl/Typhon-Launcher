package achievements

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/storage"
)

type Service struct {
	games    GameLookup
	schema   SchemaSource
	now      func() time.Time
	language func() string
	scanner  scanner
	emit     func(Event)

	mu      sync.Mutex
	store   *store
	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func New(cfg Config) (*Service, error) {
	switch {
	case cfg.ConfigDir == "" || !filepath.IsAbs(cfg.ConfigDir):
		return nil, fmt.Errorf("achievements: config dir %q is not an absolute path", cfg.ConfigDir)
	case cfg.Games == nil:
		return nil, errors.New("achievements: game lookup is required")
	case cfg.Schema == nil:
		return nil, errors.New("achievements: schema source is required")
	case cfg.Language == nil:
		return nil, errors.New("achievements: language source is required")
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	return &Service{
		games:    cfg.Games,
		schema:   cfg.Schema,
		now:      now,
		language: cfg.Language,
		scanner:  newScanner(),
		emit:     emitEvent,
		store:    openStore(cfg.ConfigDir, storage.Save),
	}, nil
}

func emitEvent(ev Event) {
	if app := application.Get(); app != nil {
		app.Event.Emit(EventName, ev)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return errors.New("achievements: already started")
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

// SessionStarted and SessionStopped make the service a library session
// watcher. The library calls them under its own lock, so the scan runs in a
// goroutine owned by the service and nothing here calls back into it.
//
//wails:ignore
func (s *Service) SessionStarted(library.Game) {}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	s.mu.Lock()
	if s.ctx == nil || s.closing {
		s.mu.Unlock()
		slog.Warn("achievements scan skipped: service is not running", "game", gameID)
		return
	}
	ctx := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.afterSession(ctx, gameID)
	}()
}

func (s *Service) afterSession(ctx context.Context, gameID string) {
	_, err := s.Scan(ctx, gameID)
	switch {
	case err == nil:
	case ctx.Err() != nil:
		slog.Info("achievements scan interrupted", "game", gameID, "error", ctx.Err())
	case errors.Is(err, ErrGameNotFound):
		slog.Info("achievements scan skipped: game left the library", "game", gameID)
	case errors.Is(err, ErrNoSteamID):
		slog.Debug("achievements scan skipped: game has no steam id", "game", gameID)
	default:
		slog.Error("achievements scan failed", "game", gameID, "error", err)
	}
}

func (s *Service) lang() string {
	if s.language() == "ru" {
		return "ru"
	}
	return "en"
}

func (s *Service) readOnly() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.broken
}

func (s *Service) record(gameID string) (record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.store.games[gameID]
	return rec.clone(), ok
}

func (s *Service) resolve(gameID string) (GameRef, error) {
	ref, ok, err := s.games.Lookup(gameID)
	switch {
	case err != nil:
		return GameRef{}, fmt.Errorf("поиск игры в библиотеке: %w", err)
	case !ok:
		return GameRef{}, ErrGameNotFound
	case !validAppID(ref.AppID):
		return GameRef{}, ErrNoSteamID
	}
	return ref, nil
}

// Scan reads the progress files of a game, merges them with what is stored
// (an achievement never becomes locked because its file vanished) and tells the
// interface about the newly unlocked ones.
//
//wails:ignore
func (s *Service) Scan(ctx context.Context, gameID string) (Progress, error) {
	return s.scan(ctx, gameID)
}

// scan treats the first look at a game as a baseline: what is already earned
// is stored without an announcement, which would otherwise list everything
// earned long ago. Only keys the existing record did not have are announced.
func (s *Service) scan(ctx context.Context, gameID string) (Progress, error) {
	if err := s.readOnly(); err != nil {
		return Progress{}, err
	}
	ref, err := s.resolve(gameID)
	if err != nil {
		return Progress{}, err
	}
	res, err := s.scanner.scan(ctx, ref)
	if err != nil {
		return Progress{}, fmt.Errorf("поиск прогресса достижений: %w", err)
	}
	for _, failure := range res.Failures {
		slog.Warn("achievements progress file skipped", "game", gameID, "error", failure)
	}
	prog, err := s.merge(gameID, ref.AppID, res)
	if err != nil {
		return Progress{}, err
	}
	if len(prog.NewKeys) > 0 && !prog.First {
		s.announce(ctx, gameID, ref.AppID, prog.NewKeys)
	}
	return prog.Progress, nil
}

type mergeResult struct {
	Progress
	First bool
}

func (s *Service) merge(gameID, appID string, res scanResult) (mergeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, had := s.store.games[gameID]
	if had && cur.AppID != appID {
		cur = record{}
	}
	next := cur.clone()
	next.AppID = appID
	if next.Unlocked == nil {
		next.Unlocked = map[string]time.Time{}
	}
	var fresh []string
	for key, at := range res.Unlocked {
		if _, ok := next.Unlocked[key]; !ok {
			fresh = append(fresh, key)
		}
		mergeKey(next.Unlocked, key, at)
	}
	sort.Strings(fresh)
	first := next.ScannedAt.IsZero()
	next.Found = next.Found || res.Found
	next.Partial = res.Partial
	next.ScannedAt = s.now()
	out := mergeResult{First: first, Progress: Progress{
		GameID: gameID, AppID: appID, Unlocked: len(next.Unlocked), NewKeys: fresh, Found: next.Found, Partial: res.Partial,
	}}
	if !first && len(fresh) == 0 && cur.Found == next.Found && cur.Partial == next.Partial && cur.AppID == next.AppID && had {
		return out, nil
	}
	if err := s.store.commit(gameID, next); err != nil {
		return mergeResult{}, err
	}
	return out, nil
}

func (s *Service) announce(ctx context.Context, gameID, appID string, keys []string) {
	st, err := s.loadSchema(ctx, gameID, appID)
	if err != nil {
		slog.Warn("achievements names are not cached", "game", gameID, "error", err)
	}
	if st.fetchErr != nil {
		slog.Warn("achievements schema unavailable for the notification", "game", gameID, "error", st.fetchErr)
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key
		if j := slices.IndexFunc(st.items, func(it SchemaItem) bool { return it.Key == key }); j >= 0 && st.items[j].Name != "" {
			names[i] = st.items[j].Name
		}
	}
	s.emit(Event{GameID: gameID, Keys: keys, Names: names})
}

type schemaState struct {
	items    []SchemaItem
	none     bool
	fetchErr error
}

// loadSchema returns the cached list while it is fresh and asks the source
// otherwise. A failed fetch comes back as fetchErr next to whatever stale list
// is stored; the returned error is a failure of the store itself.
func (s *Service) loadSchema(ctx context.Context, gameID, appID string) (schemaState, error) {
	rec, _ := s.record(gameID)
	lang := s.lang()
	cached := schemaState{items: rec.Schema, none: rec.SchemaNone}
	fresh := rec.AppID == appID && rec.SchemaLang == lang && !rec.SchemaCachedAt.IsZero() && s.now().Sub(rec.SchemaCachedAt) < schemaTTL
	if fresh {
		return cached, nil
	}
	if rec.AppID != appID {
		cached = schemaState{}
	}
	items, err := s.schema.Schema(ctx, appID, lang)
	none := false
	switch {
	case errors.Is(err, ErrNoSchema):
		items, none = nil, true
	case err != nil:
		cached.fetchErr = err
		return cached, nil
	}
	if err := s.cacheSchema(gameID, appID, lang, items, none); err != nil {
		return schemaState{}, err
	}
	return schemaState{items: items, none: none}, nil
}

func (s *Service) cacheSchema(gameID, appID, lang string, items []SchemaItem, none bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.store.games[gameID]
	if cur.AppID != appID {
		cur = record{}
	}
	next := cur.clone()
	next.AppID = appID
	next.Schema = slices.Clone(items)
	next.SchemaNone = none
	next.SchemaLang = lang
	next.SchemaCachedAt = s.now()
	return s.store.commit(gameID, next)
}

// GameAchievements returns the achievement list of a game with what the player
// has unlocked. The first call for a game also scans its progress files.
func (s *Service) GameAchievements(ctx context.Context, gameID string) (View, error) {
	return s.view(ctx, gameID, false)
}

// Rescan reads the progress files again and returns the refreshed view.
func (s *Service) Rescan(ctx context.Context, gameID string) (View, error) {
	return s.view(ctx, gameID, true)
}

func (s *Service) view(ctx context.Context, gameID string, rescan bool) (View, error) {
	if err := s.readOnly(); err != nil {
		return View{}, err
	}
	ref, err := s.resolve(gameID)
	if errors.Is(err, ErrNoSteamID) {
		return View{Source: SourceNone, Items: []Item{}}, err
	}
	if err != nil {
		return View{}, err
	}
	rec, ok := s.record(gameID)
	if rescan || !ok || rec.ScannedAt.IsZero() || rec.AppID != ref.AppID {
		if _, err := s.scan(ctx, gameID); err != nil {
			return View{}, err
		}
		rec, _ = s.record(gameID)
	}
	st, err := s.loadSchema(ctx, gameID, ref.AppID)
	if err != nil {
		return View{}, err
	}
	hasProgress := rec.Found || len(rec.Unlocked) > 0
	if st.fetchErr != nil && !hasProgress && len(st.items) == 0 {
		return View{}, fmt.Errorf("список достижений: %w", st.fetchErr)
	}
	v := buildView(ref.AppID, st.items, rec, hasProgress)
	switch {
	case st.fetchErr != nil:
		v.SchemaError = SchemaUnavailable
	case st.none && hasProgress:
		v.SchemaError = SchemaNone
	}
	return v, nil
}

func buildView(appID string, schema []SchemaItem, rec record, hasProgress bool) View {
	v := View{AppID: appID, Items: make([]Item, 0, len(schema)+len(rec.Unlocked)), Source: SourceNone, Partial: rec.Partial}
	if hasProgress {
		v.Source = SourceLocal
	}
	known := make(map[string]struct{}, len(schema))
	add := func(it Item) {
		if at, ok := rec.Unlocked[it.Key]; ok {
			it.Unlocked = true
			if !at.IsZero() {
				it.UnlockedAt = at.UTC().Format(time.RFC3339)
			}
			v.Unlocked++
		}
		v.Items = append(v.Items, it)
	}
	for _, it := range schema {
		known[it.Key] = struct{}{}
		add(Item{Key: it.Key, Name: it.Name, Description: it.Description, Hidden: it.Hidden, Icon: it.Icon, IconGray: it.IconGray})
	}
	var extra []string
	for key := range rec.Unlocked {
		if _, ok := known[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		add(Item{Key: key})
	}
	v.Total = len(v.Items)
	return v
}
