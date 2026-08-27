package metadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/titles"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultTTL     = 7 * 24 * time.Hour
	maxScreenshots = 8
	minHeroRatio   = 1.5
	candidateLimit = 10
	searchTimeout  = 30 * time.Second
	refreshTimeout = 2 * time.Minute

	maxArtBatch     = 48
	maxArtQueue     = 64
	artRetries      = 2
	artBatchTimeout = 5 * time.Minute
)

type refreshMode int

const (
	modeFull refreshMode = iota
	modeArt
)

var (
	errNoGameID   = errors.New("не указана игра")
	errNotStarted = errors.New("сервис метаданных не запущен")
	errEmptyQuery = errors.New("пустой поисковый запрос")
	errNoTitle    = errors.New("провайдер вернул игру без названия")
	errBusy       = errors.New("метаданные этой игры уже обновляются")
)

// MatchState рассказывает карточке игры, на какой стадии поиск метаданных:
// пустая карточка без этого поля неотличима от карточки, поиск для которой
// провалился ещё вчера.
type MatchState string

const (
	MatchIdle      MatchState = "idle"
	MatchSearching MatchState = "searching"
	MatchUnmatched MatchState = "unmatched"
	MatchFailed    MatchState = "failed"
	MatchSkipped   MatchState = "skipped"
)

type View struct {
	Game        catalog.Game `json:"game"`
	Cover       string       `json:"cover"`
	Hero        string       `json:"hero"`
	Screenshots []MediaAsset `json:"screenshots"`
	Resolved    bool         `json:"resolved"`
	Stale       bool         `json:"stale"`
	Provider    string       `json:"provider"`
	Match       MatchState   `json:"match"`
}

type Art struct {
	Cover string `json:"cover"`
	Hero  string `json:"hero"`
}

type Service struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	loops      sync.WaitGroup
	closing    bool
	refreshing map[string]bool

	catalog  *catalog.Service
	provider Provider
	store    *assetStore
	attempts *attemptStore
	budget   *budget
	images   *http.Client
	imgGate  chan struct{}
	artGate  chan struct{}
	ttl      time.Duration
}

func NewService(cat *catalog.Service, provider Provider) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(dir, cat, provider)
}

//wails:ignore
func NewServiceAt(dir string, cat *catalog.Service, provider Provider) (*Service, error) {
	if cat == nil {
		return nil, errors.New("каталог игр недоступен")
	}
	store, err := newAssetStore(dir)
	if err != nil {
		return nil, err
	}
	attempts, err := newAttemptStore(dir, time.Now)
	if err != nil {
		return nil, err
	}
	return &Service{
		refreshing: map[string]bool{},
		catalog:    cat,
		provider:   provider,
		store:      store,
		attempts:   attempts,
		budget:     newBudget(time.Now),
		images:     newImageClient(),
		imgGate:    make(chan struct{}, imageSlots),
		artGate:    make(chan struct{}, 1),
		ttl:        defaultTTL,
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	own, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.ctx, s.cancel = own, cancel
	s.mu.Unlock()

	if err := s.store.clearCandidates(); err != nil {
		return err
	}
	if err := s.store.sweep(ctx); err != nil {
		return err
	}
	s.loops.Add(1)
	go s.flushAttempts(own)
	return nil
}

// flushAttempts сбрасывает журнал попыток пачками: при 17 тысячах игр в
// каталоге запись файла на каждый неудачный поиск стоит дороже самого поиска.
func (s *Service) flushAttempts(ctx context.Context) {
	defer s.loops.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.attempts.flush(); err != nil {
				slog.Warn("flush metadata attempts", "error", err)
			}
		}
	}
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
	s.loops.Wait()
	return s.attempts.flush()
}

func (s *Service) Available() bool {
	return s.provider != nil
}

func (s *Service) GetView(gameID string) (View, error) {
	gameID = strings.TrimSpace(gameID)
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return View{}, err
	}
	view := s.view(game)
	if !view.Resolved && s.busy(gameID) {
		view.Match = MatchSearching
	}
	return view, nil
}

func (s *Service) busy(gameID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshing[gameID]
}

func (s *Service) GetArt(gameIDs []string) (map[string]Art, error) {
	out := make(map[string]Art, len(gameIDs))
	for _, id := range gameIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		game, err := s.catalog.GetGame(id)
		if err != nil {
			if errors.Is(err, catalog.ErrNoGame) {
				continue
			}
			return nil, err
		}
		art := Art{Cover: s.coverURL(game), Hero: heroURL(game, s.screenshots(game.ID))}
		if art.Cover == "" && art.Hero == "" {
			continue
		}
		out[id] = art
	}
	return out, nil
}

//wails:ignore
func (s *Service) CoverSourceURL(gameID string) (string, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return "", errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return "", fmt.Errorf("cover source %s: %w", gameID, err)
	}
	if asset, ok := s.store.find(game.CoverAssetID); ok && asset.GameID == game.ID {
		return asset.SourceURL, nil
	}
	for _, a := range s.store.list(game.ID) {
		if a.Type == AssetCover {
			return a.SourceURL, nil
		}
	}
	return "", nil
}

func (s *Service) SearchCandidates(query string) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errEmptyQuery
	}
	provider, session, err := s.session(searchTimeout)
	if err != nil {
		return nil, err
	}
	defer session.cancel()

	found, err := s.search(session.ctx, provider, query, candidateLimit, classUser)
	if err != nil {
		return nil, err
	}
	return s.decorate(session.ctx, rank(query, 0, found)), nil
}

func (s *Service) FindCandidates(gameID string) ([]Candidate, error) {
	game, err := s.catalog.GetGame(strings.TrimSpace(gameID))
	if err != nil {
		return nil, err
	}
	provider, session, err := s.session(searchTimeout)
	if err != nil {
		return nil, err
	}
	defer session.cancel()

	found, err := s.search(session.ctx, provider, game.Title, candidateLimit, classUser)
	if err != nil {
		return nil, err
	}
	return s.decorate(session.ctx, rank(game.Title, year(game), found)), nil
}

func (s *Service) ApplyMatch(gameID, providerID string) (View, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return View{}, ErrNoMatch
	}
	return s.runRefresh(strings.TrimSpace(gameID), providerID)
}

func (s *Service) Refresh(gameID string) (View, error) {
	return s.runRefresh(strings.TrimSpace(gameID), "")
}

// DismissMatch — это «забить»: пользователь согласился жить без описания и
// обложки, и ни фон, ни открытая карточка больше об этой игре не спрашивают.
func (s *Service) DismissMatch(gameID string) (View, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return View{}, errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return View{}, err
	}
	prev, existed := s.attempts.dismiss(gameID)
	if err := s.attempts.flush(); err != nil {
		s.attempts.restore(gameID, prev, existed)
		return View{}, err
	}
	view := s.view(game)
	emit("metadata:updated", view)
	return view, nil
}

func (s *Service) EnsureFresh(gameID string) (bool, error) {
	gameID = strings.TrimSpace(gameID)
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return false, err
	}
	if s.provider == nil {
		return false, nil
	}
	providerID := game.ExternalIDs.IGDB
	if providerID != "" && !s.stale(game) && !game.MetadataPartial {
		return false, nil
	}

	s.mu.Lock()
	if s.closing || s.ctx == nil {
		s.mu.Unlock()
		return false, errNotStarted
	}
	if s.refreshing[gameID] || !s.attempts.due(gameID, true) {
		s.mu.Unlock()
		return false, nil
	}
	s.refreshing[gameID] = true
	s.wg.Add(1)
	s.mu.Unlock()

	s.spawn(game, providerID, modeFull, classUser)
	return true, nil
}

func (s *Service) EnsureArt(gameIDs []string) ([]string, error) {
	if len(gameIDs) > maxArtBatch {
		gameIDs = gameIDs[:maxArtBatch]
	}
	accepted := make([]string, 0, len(gameIDs))
	batch := make([]catalog.Game, 0, len(gameIDs))
	for _, id := range gameIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		game, err := s.catalog.GetGame(id)
		if err != nil {
			if errors.Is(err, catalog.ErrNoGame) {
				accepted = append(accepted, id)
				continue
			}
			return accepted, err
		}
		if s.provider == nil || (s.coverURL(game) != "" && !s.stale(game)) {
			accepted = append(accepted, id)
			continue
		}
		taken, queued, err := s.takeArt(game)
		if err != nil {
			return accepted, err
		}
		if !taken {
			continue
		}
		accepted = append(accepted, id)
		if queued {
			batch = append(batch, game)
		}
	}
	if len(batch) > 0 {
		s.wg.Add(1)
		go s.runArtBatch(batch)
	}
	return accepted, nil
}

func (s *Service) takeArt(game catalog.Game) (taken, queued bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ctx == nil {
		return false, false, errNotStarted
	}
	if s.refreshing[game.ID] || !s.attempts.due(game.ID, false) {
		return true, false, nil
	}
	if len(s.refreshing) >= maxArtQueue {
		return false, false, nil
	}
	s.refreshing[game.ID] = true
	return true, true, nil
}

func (s *Service) runArtBatch(games []catalog.Game) {
	defer s.wg.Done()
	defer func() {
		for _, game := range games {
			s.release(game.ID)
		}
	}()

	ctx := s.baseContext()
	if ctx == nil {
		return
	}
	select {
	case s.artGate <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-s.artGate }()

	rest := games
	if resolver, ok := s.provider.(BatchProvider); ok {
		rest = s.resolveArt(ctx, resolver, games)
	}
	for _, game := range rest {
		if ctx.Err() != nil {
			return
		}
		s.runArt(game)
	}
}

// runArt повторяет попытку один раз: паузу после 429 уже выдержал бюджет, а
// второй отказ подряд означает, что сервер занят надолго и ждать его нечем.
func (s *Service) runArt(game catalog.Game) {
	for attempt := range artRetries {
		view, err := s.refresh(game.ID, game.ExternalIDs.IGDB, modeArt, classBackground)
		if isRateLimited(err) && attempt+1 < artRetries {
			continue
		}
		s.report(game, view, err)
		return
	}
}

// resolveArt разбирает пачку одним запросом и возвращает игры, которым ответ
// ничего не дал: их добивает поштучный поиск.
func (s *Service) resolveArt(ctx context.Context, resolver BatchProvider, games []catalog.Game) []catalog.Game {
	titles := make([]string, 0, len(games))
	for _, game := range games {
		titles = append(titles, game.Title)
	}
	resolved, err := s.resolve(ctx, resolver, titles)
	if err != nil {
		slog.Warn("batch metadata lookup", "titles", len(titles), "error", err)
		return games
	}

	byTitle := make(map[string]GameMetadata, len(resolved))
	for _, item := range resolved {
		byTitle[strings.ToLower(strings.TrimSpace(item.Title))] = item.Meta
	}

	applyCtx, cancel := context.WithTimeout(ctx, artBatchTimeout)
	defer cancel()

	rest := make([]catalog.Game, 0, len(games))
	for _, game := range games {
		meta, ok := byTitle[strings.ToLower(strings.TrimSpace(game.Title))]
		if !ok {
			rest = append(rest, game)
			continue
		}
		if !sameGame(game, meta) {
			slog.Info("batch metadata rejected", "game", game.Title, "offered", meta.Title)
			rest = append(rest, game)
			continue
		}
		view, err := s.apply(applyCtx, game, meta, modeArt)
		if err != nil {
			s.report(game, view, err)
			continue
		}
		s.report(game, view, nil)
	}
	return rest
}

// Пачка приходит от чужого матчера: бэкенд отвечает на название лучшим
// кандидатом, а не точным совпадением. Поштучный путь режет такие ответы
// порогом pick, у пачки своего порога не было.
func sameGame(game catalog.Game, meta GameMetadata) bool {
	if id := strings.TrimSpace(game.ExternalIDs.IGDB); id != "" && id == strings.TrimSpace(meta.ProviderID) {
		return true
	}
	return titles.Similarity(game.Title, meta.Title) >= autoThreshold
}

func (s *Service) resolve(ctx context.Context, resolver BatchProvider, titles []string) ([]Resolved, error) {
	if err := s.budget.acquire(ctx, classBackground); err != nil {
		return nil, err
	}
	defer s.budget.release(classBackground)

	resolved, err := resolver.Resolve(ctx, titles)
	if err != nil {
		return nil, s.notePenalty(err)
	}
	return resolved, nil
}

func (s *Service) spawn(game catalog.Game, providerID string, mode refreshMode, class callClass) {
	go func() {
		defer s.wg.Done()
		defer s.release(game.ID)
		view, err := s.refresh(game.ID, providerID, mode, class)
		s.report(game, view, err)
	}()
}

// report решает судьбу игры до следующей попытки. Отказ по лимиту — не вина
// игры: он не увеличивает backoff, иначе один занятый сервер похоронил бы
// весь каталог на сутки.
func (s *Service) report(game catalog.Game, view View, err error) {
	switch {
	case err == nil:
		s.budget.reward()
		s.attempts.succeed(game.ID)
		emit("metadata:updated", view)
	case isRateLimited(err), errors.Is(err, errBudgetBusy),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		slog.Debug("metadata refresh postponed", "game", game.ID)
		s.emitState(game, MatchFailed)
	case errors.Is(err, ErrAmbiguous), errors.Is(err, ErrNoMatch):
		s.attempts.fail(game.ID, attemptUnmatched)
		slog.Info("metadata match needs a manual choice", "game", game.ID, "title", game.Title)
		s.emitState(game, MatchUnmatched)
	default:
		s.attempts.fail(game.ID, attemptTransient)
		slog.Warn("background metadata refresh", "game", game.ID, "error", err)
		s.emitState(game, MatchFailed)
	}
}

// emitState говорит открытой карточке, чем кончился поиск. Без него страница,
// показавшая «ищем метаданные», остаётся с этой надписью навсегда: успешный
// поиск событие шлёт, а неудачный до сих пор не слал ничего.
func (s *Service) emitState(game catalog.Game, state MatchState) {
	view := s.view(game)
	if view.Resolved {
		return
	}
	view.Match = state
	emit("metadata:updated", view)
}

func (s *Service) baseContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil
	}
	return s.ctx
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	_, limited := retryAfter(err)
	return limited
}

func (s *Service) notePenalty(err error) error {
	wait, limited := retryAfter(err)
	if !limited {
		return err
	}
	pause := s.budget.penalize(wait)
	rate, _, penalties, _ := s.budget.stats()
	slog.Warn("metadata provider rate limited", "pause", pause, "rate", rate, "penalties", penalties)
	return err
}

func (s *Service) search(ctx context.Context, provider Provider, query string, limit int, class callClass) ([]Candidate, error) {
	if err := s.budget.acquire(ctx, class); err != nil {
		return nil, err
	}
	defer s.budget.release(class)

	found, err := provider.Search(ctx, query, limit)
	if err != nil {
		return nil, s.notePenalty(err)
	}
	return found, nil
}

func (s *Service) fetchMeta(ctx context.Context, provider Provider, providerID string, class callClass) (GameMetadata, error) {
	if err := s.budget.acquire(ctx, class); err != nil {
		return GameMetadata{}, err
	}
	defer s.budget.release(class)

	meta, err := provider.Get(ctx, providerID)
	if err != nil {
		return GameMetadata{}, s.notePenalty(err)
	}
	return meta, nil
}

// Картинки лежат на CDN провайдера, а не за нашим API: у них свой лимит и своя
// очередь, иначе девять скриншотов съедают бюджет запросов к метаданным.
func (s *Service) fetchImage(ctx context.Context, url string) (fetchedImage, error) {
	select {
	case s.imgGate <- struct{}{}:
	case <-ctx.Done():
		return fetchedImage{}, ctx.Err()
	}
	defer func() { <-s.imgGate }()

	return fetchImage(ctx, s.images, url)
}

func (s *Service) runRefresh(gameID, providerID string) (View, error) {
	if gameID == "" {
		return View{}, errNoGameID
	}
	s.mu.Lock()
	if s.closing || s.ctx == nil {
		s.mu.Unlock()
		return View{}, errNotStarted
	}
	if s.refreshing[gameID] {
		s.mu.Unlock()
		return View{}, errBusy
	}
	s.refreshing[gameID] = true
	s.mu.Unlock()
	defer s.release(gameID)

	s.attempts.resume(gameID)

	view, err := s.refresh(gameID, providerID, modeFull, classUser)
	if err != nil {
		return View{}, err
	}
	s.budget.reward()
	s.attempts.succeed(gameID)
	emit("metadata:updated", view)
	return view, nil
}

func (s *Service) release(gameID string) {
	s.mu.Lock()
	delete(s.refreshing, gameID)
	s.mu.Unlock()
}

func (s *Service) refresh(gameID, providerID string, mode refreshMode, class callClass) (View, error) {
	game, err := s.catalog.GetGame(gameID)
	if err != nil {
		return View{}, err
	}
	provider, session, err := s.session(refreshTimeout)
	if err != nil {
		return View{}, err
	}
	defer session.cancel()
	ctx := session.ctx

	meta, err := s.lookup(ctx, provider, game, providerID, class)
	if err != nil {
		return View{}, err
	}
	return s.apply(ctx, game, meta, mode)
}

func (s *Service) lookup(ctx context.Context, provider Provider, game catalog.Game, providerID string, class callClass) (GameMetadata, error) {
	if providerID == "" {
		providerID = game.ExternalIDs.IGDB
	}
	if providerID == "" {
		found, err := s.search(ctx, provider, game.Title, candidateLimit, class)
		if err != nil {
			return GameMetadata{}, err
		}
		chosen, err := pick(game.Title, year(game), found)
		if err != nil {
			return GameMetadata{}, err
		}
		providerID = chosen.ProviderID
	}
	return s.fetchMeta(ctx, provider, providerID, class)
}

func (s *Service) apply(ctx context.Context, game catalog.Game, meta GameMetadata, mode refreshMode) (View, error) {
	if strings.TrimSpace(meta.Title) == "" {
		return View{}, errNoTitle
	}
	gameID := game.ID

	batch, err := s.buildAssets(ctx, game, meta, mode)
	if err != nil {
		return View{}, err
	}

	previous, err := s.store.replace(gameID, batch.assets)
	if err != nil {
		s.store.removeFiles(batch.created)
		return View{}, err
	}

	updated, err := s.catalog.ApplyMetadata(gameID, catalog.MetadataPatch{
		IGDBID:       meta.ProviderID,
		Title:        meta.Title,
		Summary:      meta.Summary,
		ReleaseDate:  meta.ReleaseDate,
		Developer:    meta.Developer,
		Publisher:    meta.Publisher,
		Genres:       meta.Genres,
		Themes:       meta.Themes,
		Platforms:    meta.Platforms,
		CoverAssetID: batch.cover,
		HeroAssetID:  batch.hero,
		UpdatedAt:    time.Now(),
		Partial:      batch.partial,
	})
	if err != nil {
		if _, restoreErr := s.store.replace(gameID, stripURLs(previous)); restoreErr != nil {
			slog.Error("restore media assets", "game", gameID, "error", restoreErr)
		}
		s.store.removeFiles(batch.created)
		return View{}, err
	}

	s.store.removeFiles(staleAssets(previous, batch.assets))
	return s.view(updated), nil
}

type assetBatch struct {
	assets  []MediaAsset
	created []MediaAsset
	cover   string
	hero    string
	partial bool
}

func (s *Service) buildAssets(ctx context.Context, game catalog.Game, meta GameMetadata, mode refreshMode) (assetBatch, error) {
	previous := s.store.list(game.ID)
	batch := assetBatch{}

	if meta.Cover != nil && meta.Cover.URL != "" {
		asset, err := s.storeImage(ctx, game.ID, AssetCover, meta.Cover.URL)
		switch {
		case err == nil:
			batch.assets = append(batch.assets, asset)
			batch.created = append(batch.created, asset)
			batch.cover = asset.ID
		case ctx.Err() != nil:
			return assetBatch{}, ctx.Err()
		default:
			slog.Warn("download cover", "game", game.ID, "url", meta.Cover.URL, "error", err)
		}
	}
	if batch.cover == "" {
		if kept, ok := carryCover(previous, game.CoverAssetID); ok {
			batch.assets = append(batch.assets, kept)
			batch.cover = kept.ID
		}
	}

	if mode == modeArt {
		shots := carryScreenshots(previous)
		batch.assets = append(batch.assets, shots...)
		batch.hero = heroOf(shots)
		batch.partial = len(shots) == 0
		return batch, nil
	}

	limit := min(len(meta.Screenshots), maxScreenshots)
	var shots []MediaAsset
	for _, shot := range meta.Screenshots[:limit] {
		if shot.URL == "" {
			continue
		}
		asset, err := s.storeImage(ctx, game.ID, AssetScreenshot, shot.URL)
		if err != nil {
			if ctx.Err() != nil {
				return assetBatch{}, ctx.Err()
			}
			slog.Warn("download screenshot", "game", game.ID, "url", shot.URL, "error", err)
			continue
		}
		shots = append(shots, asset)
		batch.created = append(batch.created, asset)
	}
	if len(shots) == 0 {
		shots = carryScreenshots(previous)
	}
	batch.assets = append(batch.assets, shots...)
	batch.hero = heroOf(shots)
	return batch, nil
}

func (s *Service) storeImage(ctx context.Context, gameID string, kind AssetType, url string) (MediaAsset, error) {
	img, err := s.fetchImage(ctx, url)
	if err != nil {
		return MediaAsset{}, err
	}
	id, err := newAssetID()
	if err != nil {
		return MediaAsset{}, err
	}
	rel, err := writeAsset(s.store.mediaRoot(), gameID, id, img)
	if err != nil {
		return MediaAsset{}, err
	}
	return MediaAsset{
		ID:        id,
		GameID:    gameID,
		Type:      kind,
		SourceURL: url,
		Path:      rel,
		Width:     img.Width,
		Height:    img.Height,
		CreatedAt: time.Now(),
	}, nil
}

func (s *Service) decorate(ctx context.Context, candidates []Candidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.ThumbURL != "" {
			thumb, err := s.cacheThumb(ctx, c.ThumbURL)
			if err != nil {
				slog.Debug("cache candidate thumb", "url", c.ThumbURL, "error", err)
			} else {
				c.Thumb = thumb
			}
		}
		out = append(out, c)
	}
	return out
}

func (s *Service) cacheThumb(ctx context.Context, url string) (string, error) {
	dir := s.store.candidatesRoot()
	name := thumbName(url)
	for _, ext := range []string{".jpg", ".png"} {
		_, err := os.Stat(filepath.Join(dir, name+ext))
		if err == nil {
			return thumbURL(name + ext), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat candidate thumb: %w", err)
		}
	}

	img, err := s.fetchImage(ctx, url)
	if err != nil {
		return "", err
	}
	ext, err := thumbExt(img.Format)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create candidate cache: %w", err)
	}
	if err := storage.WriteAtomic(filepath.Join(dir, name+ext), img.Data); err != nil {
		return "", err
	}
	return thumbURL(name + ext), nil
}

func (s *Service) view(game catalog.Game) View {
	view := View{
		Game:        game,
		Cover:       s.coverURL(game),
		Screenshots: s.screenshots(game.ID),
		Resolved:    game.ExternalIDs.IGDB != "",
		Stale:       s.stale(game),
	}
	if s.provider != nil {
		view.Provider = s.provider.Name()
	}
	view.Hero = heroURL(game, view.Screenshots)
	view.Match = s.matchState(game)
	return view
}

func (s *Service) matchState(game catalog.Game) MatchState {
	if s.provider == nil || game.ExternalIDs.IGDB != "" {
		return MatchIdle
	}
	rec, ok := s.attempts.state(game.ID)
	switch {
	case !ok:
		return MatchIdle
	case rec.Dismissed:
		return MatchSkipped
	case rec.Kind == attemptUnmatched:
		return MatchUnmatched
	default:
		return MatchFailed
	}
}

func (s *Service) screenshots(gameID string) []MediaAsset {
	var shots []MediaAsset
	for _, a := range s.store.list(gameID) {
		if a.Type == AssetScreenshot {
			shots = append(shots, a)
		}
	}
	return shots
}

func (s *Service) coverURL(game catalog.Game) string {
	if asset, ok := s.store.find(game.CoverAssetID); ok && asset.GameID == game.ID {
		return asset.URL
	}
	for _, a := range s.store.list(game.ID) {
		if a.Type == AssetCover {
			return a.URL
		}
	}
	return ""
}

func (s *Service) stale(game catalog.Game) bool {
	if game.MetadataUpdatedAt == nil {
		return true
	}
	return time.Since(*game.MetadataUpdatedAt) > s.ttl
}

type providerSession struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *Service) session(timeout time.Duration) (Provider, providerSession, error) {
	if s.provider == nil {
		return nil, providerSession{}, ErrNotConfigured
	}
	s.mu.Lock()
	base := s.ctx
	closing := s.closing
	s.mu.Unlock()
	if base == nil || closing {
		return nil, providerSession{}, errNotStarted
	}
	ctx, cancel := context.WithTimeout(base, timeout)
	return s.provider, providerSession{ctx: ctx, cancel: cancel}, nil
}

func thumbName(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:8])
}

func thumbURL(name string) string {
	return "/" + path.Join(mediaDirName, candidatesDirName, name)
}

func thumbExt(format string) (string, error) {
	switch format {
	case "jpeg":
		return ".jpg", nil
	case "png":
		return ".png", nil
	default:
		return "", fmt.Errorf("%w: %s", errImageFormat, format)
	}
}

func year(game catalog.Game) int {
	if game.ReleaseYear == nil {
		return 0
	}
	return *game.ReleaseYear
}

func landscape(a MediaAsset) bool {
	return a.Height > 0 && float64(a.Width)/float64(a.Height) >= minHeroRatio
}

func heroURL(game catalog.Game, shots []MediaAsset) string {
	for _, a := range shots {
		if a.ID == game.HeroAssetID {
			return a.URL
		}
	}
	for _, a := range shots {
		if landscape(a) {
			return a.URL
		}
	}
	return ""
}

func heroOf(shots []MediaAsset) string {
	for _, a := range shots {
		if landscape(a) {
			return a.ID
		}
	}
	return ""
}

func carryCover(previous []MediaAsset, coverID string) (MediaAsset, bool) {
	for _, a := range previous {
		if a.Type == AssetCover && a.ID == coverID {
			return a, true
		}
	}
	for _, a := range previous {
		if a.Type == AssetCover {
			return a, true
		}
	}
	return MediaAsset{}, false
}

func carryScreenshots(previous []MediaAsset) []MediaAsset {
	var out []MediaAsset
	for _, a := range previous {
		if a.Type == AssetScreenshot {
			out = append(out, a)
		}
	}
	return out
}

func staleAssets(previous, next []MediaAsset) []MediaAsset {
	keep := make(map[string]bool, len(next))
	for _, a := range next {
		keep[a.ID] = true
	}
	var out []MediaAsset
	for _, a := range previous {
		if !keep[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

func stripURLs(assets []MediaAsset) []MediaAsset {
	out := make([]MediaAsset, 0, len(assets))
	for _, a := range assets {
		a.URL = ""
		out = append(out, a)
	}
	return out
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}
