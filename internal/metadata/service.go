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
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultTTL     = 7 * 24 * time.Hour
	maxScreenshots = 8
	// Artworks only feed the hero choice, which needs one picture; the second
	// stands in when the first fails to download or does not fit the band.
	maxArtworks    = 2
	candidateLimit = 10
	searchTimeout  = 30 * time.Second

	maxCoverCandidates = 3

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
	errNoGameID   = uierr.New("metadata.no_game_id", "не указана игра")
	errNotStarted = uierr.New("metadata.not_started", "сервис метаданных не запущен")
	errEmptyQuery = uierr.New("metadata.empty_query", "пустой поисковый запрос")
	errNoTitle    = uierr.New("metadata.no_title", "провайдер вернул игру без названия")
	errBusy       = uierr.New("metadata.busy", "метаданные этой игры уже обновляются")
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
	Game         catalog.Game      `json:"game"`
	Cover        string            `json:"cover"`
	Hero         string            `json:"hero"`
	HeroIsPoster bool              `json:"heroIsPoster"`
	Screenshots  []MediaAsset      `json:"screenshots"`
	Resolved     bool              `json:"resolved"`
	Stale        bool              `json:"stale"`
	Provider     string            `json:"provider"`
	Match        MatchState        `json:"match"`
	StoreLinks   map[string]string `json:"storeLinks,omitempty"`
	Steam        *SteamInfo        `json:"steam,omitempty"`
}

type Art struct {
	Cover        string `json:"cover"`
	Hero         string `json:"hero"`
	HeroIsPoster bool   `json:"heroIsPoster"`
}

type Service struct {
	language      string
	languageEpoch uint64
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	loops         sync.WaitGroup
	closing       bool
	refreshing    map[string]bool

	catalog       *catalog.Service
	applyMetadata func(string, catalog.MetadataPatch) (catalog.Game, error)
	openURL       func(string) error
	provider      Provider
	store         *assetStore
	attempts      *attemptStore
	steam         *steamStore
	saves         *savesCache
	clock         func() time.Time
	budget        *budget
	images        *http.Client
	imgGate       chan struct{}
	artGate       chan struct{}
	ttl           time.Duration
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
	steam, err := newSteamStore(dir, time.Now())
	if err != nil {
		return nil, err
	}
	saves, err := newSavesCache(dir)
	if err != nil {
		return nil, err
	}
	return &Service{
		steam:         steam,
		saves:         saves,
		clock:         time.Now,
		refreshing:    map[string]bool{},
		catalog:       cat,
		applyMetadata: cat.ApplyMetadata,
		openURL:       openSystemBrowser,
		provider:      provider,
		store:         store,
		attempts:      attempts,
		budget:        newBudget(time.Now),
		images:        newImageClient(),
		imgGate:       make(chan struct{}, imageSlots),
		artGate:       make(chan struct{}, 1),
		ttl:           defaultTTL,
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
	if s.busy(gameID) {
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
		hero := s.hero(game)
		art := Art{Cover: s.coverURL(game), Hero: hero.asset.URL, HeroIsPoster: hero.poster}
		if art.Cover == "" && art.Hero == "" {
			continue
		}
		out[id] = art
	}
	return out, nil
}

// LocalArt holds absolute paths of the cover and hero image files of a game; an
// image that is not stored locally is an empty string.
type LocalArt struct {
	Cover string
	Hero  string
}

// ArtFiles picks the same cover and hero as GetArt but returns the files on
// disk, for consumers that need to copy the images rather than show them.
//
//wails:ignore
func (s *Service) ArtFiles(gameID string) (LocalArt, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return LocalArt{}, errNoGameID
	}
	game, err := s.catalog.GetGame(gameID)
	if errors.Is(err, catalog.ErrNoGame) {
		return LocalArt{}, nil
	}
	if err != nil {
		return LocalArt{}, fmt.Errorf("art files %s: %w", gameID, err)
	}
	var art LocalArt
	if asset, ok := s.coverAsset(game); ok {
		if art.Cover, err = s.assetFile(asset); err != nil {
			return LocalArt{}, err
		}
	}
	// The Steam grid's hero slot is wide; a poster there would be stretched,
	// and the grid already shows the cover in its portrait slot.
	if hero := s.hero(game); hero.ok && !hero.poster {
		if art.Hero, err = s.assetFile(hero.asset); err != nil {
			return LocalArt{}, err
		}
	}
	return art, nil
}

func (s *Service) assetFile(asset MediaAsset) (string, error) {
	full, err := assetPath(s.store.mediaRoot(), asset.Path)
	if err != nil {
		return "", fmt.Errorf("asset %s: %w", asset.ID, err)
	}
	if _, err := os.Stat(full); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("stat asset %s: %w", asset.ID, err)
	}
	return full, nil
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

// FindCover returns the cover of the game with exactly this title whose
// platforms accept approves, or ErrNoMatch. It goes through the request budget
// as a user's search and ends with ctx or the service.
//
//wails:ignore
func (s *Service) FindCover(ctx context.Context, title string, accept func(platforms []string) bool) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errEmptyQuery
	}
	provider, session, err := s.session(searchTimeout)
	if err != nil {
		return "", err
	}
	defer session.cancel()
	ctx, cancel := context.WithTimeout(requestLanguage(ctx, s.currentLanguage()), searchTimeout)
	defer cancel()
	//nolint:contextcheck // invariant 19: only watches the service context, so the lookup ends with the service
	defer context.AfterFunc(session.ctx, cancel)()

	found, err := s.search(ctx, provider, title, candidateLimit, classUser)
	if err != nil {
		return "", err
	}
	wanted := titles.Normalize(title)
	checked := 0
	for _, candidate := range found {
		if checked == maxCoverCandidates {
			break
		}
		if titles.Normalize(candidate.Title) != wanted {
			continue
		}
		checked++
		meta, err := s.fetchMeta(ctx, provider, candidate.ProviderID, classUser)
		if err != nil {
			return "", err
		}
		if accept(meta.Platforms) && meta.Cover != nil && meta.Cover.URL != "" {
			return meta.Cover.URL, nil
		}
	}
	return "", ErrNoMatch
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
	if (providerID != "" || game.ExternalIDs.Steam != "") && !s.stale(game) && !game.MetadataPartial && !s.needsSteam(game) {
		return false, nil
	}

	s.mu.Lock()
	if s.closing || s.ctx == nil {
		s.mu.Unlock()
		return false, errNotStarted
	}
	// Backoff and dismissal apply even before the first metadata language has
	// been recorded. Calling due outside the language equality check prevents
	// an empty MetadataLanguage from turning every card open into a new search.
	if s.refreshing[gameID] || !s.attempts.due(gameID, true) {
		s.mu.Unlock()
		return false, nil
	}
	s.refreshing[gameID] = true
	s.wg.Add(1)
	s.mu.Unlock()

	s.spawn(game, providerID, modeFull, refreshClass(game))
	return true, nil
}

// A game that already shows complete metadata and is only due for the TTL or
// language refresh must not compete with the user's own requests: the page is
// usable as it is, so this refresh may yield and wait for a quieter moment.
func refreshClass(game catalog.Game) callClass {
	resolved := game.ExternalIDs.IGDB != "" || game.ExternalIDs.Steam != ""
	if resolved && game.MetadataUpdatedAt != nil && !game.MetadataPartial {
		return classBackground
	}
	return classUser
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
	if ctx != nil {
		ctx = requestLanguage(ctx, s.currentLanguage())
	}
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
		if !sameGame(game, meta) || (s.catalog.HasRemoteCatalog() && game.ExternalIDs.IGDB == "" && game.ExternalIDs.Steam == "") {
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
		view, err := s.refresh(game.ID, providerID, mode, class)
		s.release(game.ID)
		if errors.Is(err, context.Canceled) && s.baseContext() != nil && s.baseContext().Err() == nil {
			if started, retryErr := s.EnsureFresh(game.ID); retryErr != nil {
				slog.Debug("retry metadata after locale change", "gameID", game.ID, "error", retryErr)
			} else if started {
				return
			}
		}
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
	case errors.Is(err, errBudgetBusy) && game.MetadataUpdatedAt != nil:
		// The page already shows this game's metadata, so a refresh that yielded is not a
		// failure; the event only clears the "searching" state set by EnsureFresh.
		slog.Debug("metadata refresh yielded", "game", game.ID)
		current, getErr := s.catalog.GetGame(game.ID)
		if getErr != nil {
			slog.Debug("metadata refresh yielded: game is gone", "game", game.ID, "error", getErr)
			return
		}
		emit("metadata:updated", s.view(current))
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

// FetchGame asks the provider about one game on the user's behalf, under the
// same budget and 429 pause as a card opened in the window.
//
//wails:ignore
func (s *Service) FetchGame(ctx context.Context, providerID string) (GameMetadata, error) {
	if s.provider == nil {
		return GameMetadata{}, ErrNotConfigured
	}
	return s.fetchMeta(ctx, s.provider, providerID, classUser)
}

// SearchTitle looks a title up on the user's behalf, under the same budget and
// 429 pause as a search in the window.
//
//wails:ignore
func (s *Service) SearchTitle(ctx context.Context, title string) ([]Candidate, error) {
	if s.provider == nil {
		return nil, ErrNotConfigured
	}
	return s.search(ctx, s.provider, title, candidateLimit, classUser)
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
	if providerID == "" && game.ExternalIDs.Steam != "" {
		providerID = "steam:" + game.ExternalIDs.Steam
	}
	if providerID == "" {
		if s.catalog.HasRemoteCatalog() {
			return GameMetadata{}, ErrAmbiguous
		}
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

	// Do not let an old-language response overwrite a newer UI selection.
	// Keep the metadata mutex out of disk replacement and catalog persistence:
	// readers and a locale change must remain responsive while those operations
	// fsync.
	s.mu.Lock()
	lang := s.language
	if lang != "ru" {
		lang = "en"
	}
	epoch := s.languageEpoch
	s.mu.Unlock()
	if Language(ctx) != lang {
		s.store.removeFiles(batch.created)
		return View{}, context.Canceled
	}
	previous, err := s.store.replace(gameID, batch.assets)
	if err != nil {
		s.store.removeFiles(batch.created)
		return View{}, err
	}
	s.mu.Lock()
	currentLanguage := s.language
	if currentLanguage != "ru" {
		currentLanguage = "en"
	}
	languageChanged := s.closing || Language(ctx) != currentLanguage || epoch != s.languageEpoch
	s.mu.Unlock()
	if languageChanged {
		if _, restoreErr := s.store.replace(gameID, stripURLs(previous)); restoreErr != nil {
			slog.Error("restore media assets", "game", gameID, "error", restoreErr)
		}
		s.store.removeFiles(batch.created)
		return View{}, context.Canceled
	}

	igdbID, steamID := matchIDs(meta)
	applyMetadata := s.applyMetadata
	if applyMetadata == nil {
		applyMetadata = s.catalog.ApplyMetadata
	}
	var restoreSteam func() error
	if mode == modeFull || meta.Steam != nil {
		info, ok := SanitizeSteamInfo(meta.Steam)
		_, had := s.steam.get(gameID)
		// A block that failed validation must not overwrite a valid stored one;
		// without a stored one it is recorded as "none" so the card is not
		// refetched on every open.
		if ok || meta.Steam == nil || !had {
			restoreSteam, err = s.steam.put(gameID, info, s.clock())
			if err != nil {
				if _, restoreErr := s.store.replace(gameID, stripURLs(previous)); restoreErr != nil {
					slog.Error("restore media assets", "game", gameID, "error", restoreErr)
				}
				s.store.removeFiles(batch.created)
				return View{}, err
			}
		}
	}
	updated, err := applyMetadata(gameID, catalog.MetadataPatch{
		SteamID:      steamID,
		Language:     Language(ctx),
		IGDBID:       igdbID,
		Title:        meta.Title,
		Summary:      meta.Summary,
		ReleaseDate:  meta.ReleaseDate,
		Developer:    meta.Developer,
		Publisher:    meta.Publisher,
		Genres:       meta.Genres,
		Themes:       meta.Themes,
		Platforms:    meta.Platforms,
		GameType:     meta.GameType,
		ParentID:     meta.ParentID,
		StoreLinks:   meta.StoreLinks,
		CoverAssetID: batch.cover,
		HeroAssetID:  batch.hero,
		UpdatedAt:    time.Now(),
		Partial:      batch.partial || meta.DetailsPending,
	})
	if err != nil {
		if _, restoreErr := s.store.replace(gameID, stripURLs(previous)); restoreErr != nil {
			slog.Error("restore media assets", "game", gameID, "error", restoreErr)
		}
		s.store.removeFiles(batch.created)
		if restoreSteam != nil {
			if restoreErr := restoreSteam(); restoreErr != nil {
				err = errors.Join(err, restoreErr)
			}
		}
		return View{}, err
	}
	s.mu.Lock()
	languageChanged = s.closing || epoch != s.languageEpoch
	s.mu.Unlock()
	if languageChanged {
		// The catalog row is intentionally left tagged with the language that
		// produced it. stale() will schedule a fresh request for the new locale;
		// returning no view here prevents an in-flight old-language response from
		// replacing the user's current card.
		return View{}, context.Canceled
	}

	s.store.removeFiles(staleAssets(previous, batch.assets))
	s.mu.Lock()
	languageChanged = s.closing || epoch != s.languageEpoch
	s.mu.Unlock()
	if languageChanged {
		return View{}, context.Canceled
	}
	return s.view(updated), nil
}

// matchIDs splits the provider's answer into the catalog's IGDB and Steam ids;
// a Steam-only answer carries its app id in ProviderID as "steam:<id>".
func matchIDs(meta GameMetadata) (igdbID, steamID string) {
	igdbID, steamID = meta.ProviderID, meta.SteamAppID
	if strings.HasPrefix(igdbID, "steam:") {
		steamID = strings.TrimPrefix(igdbID, "steam:")
		igdbID = ""
	}
	return igdbID, steamID
}

// sameMatch reports whether meta answers for the game the stored pictures
// were downloaded for.
func sameMatch(game catalog.Game, meta GameMetadata) bool {
	igdbID, steamID := matchIDs(meta)
	if igdbID != "" && game.ExternalIDs.IGDB != "" {
		return igdbID == game.ExternalIDs.IGDB
	}
	return steamID != "" && steamID == game.ExternalIDs.Steam
}

type assetBatch struct {
	assets  []MediaAsset
	created []MediaAsset
	cover   string
	hero    string
	partial bool
}

type storedImage struct {
	asset MediaAsset
	err   error
}

// storeImages downloads every url at once and returns the results in the
// order given. The page shows nothing new until the whole batch is stored, so
// one image after another made it wait the sum of all CDN round trips; the
// image gate still caps how many downloads run together.
func (s *Service) storeImages(ctx context.Context, gameID string, kinds []AssetType, urls []string) []storedImage {
	out := make([]storedImage, len(urls))
	var wg sync.WaitGroup
	for i := range urls {
		wg.Go(func() {
			out[i].asset, out[i].err = s.storeImage(ctx, gameID, kinds[i], urls[i])
		})
	}
	wg.Wait()
	return out
}

func (s *Service) buildAssets(ctx context.Context, game catalog.Game, meta GameMetadata, mode refreshMode) (assetBatch, error) {
	previous := s.store.list(game.ID)
	batch := assetBatch{}

	var kinds []AssetType
	var urls []string
	hasCover := meta.Cover != nil && meta.Cover.URL != ""
	if hasCover {
		kinds, urls = append(kinds, AssetCover), append(urls, meta.Cover.URL)
	}
	shotRefs := meta.Screenshots[:min(len(meta.Screenshots), maxScreenshots)]
	artRefs := meta.Artworks[:min(len(meta.Artworks), maxArtworks)]
	carried := append(carryAssets(previous, AssetScreenshot), carryAssets(previous, AssetArtwork)...)
	if mode != modeArt {
		for _, shot := range shotRefs {
			if shot.URL != "" {
				kinds, urls = append(kinds, AssetScreenshot), append(urls, shot.URL)
			}
		}
		for _, art := range artRefs {
			if art.URL != "" {
				kinds, urls = append(kinds, AssetArtwork), append(urls, art.URL)
			}
		}
	} else if !hasHero(game, carried) {
		// The art refresh is what keeps library games current, so it also
		// fetches the one picture the hero needs; the gallery is left to the
		// full refresh.
		if kind, ref, ok := heroRef(shotRefs, artRefs); ok {
			kinds, urls = append(kinds, kind), append(urls, ref.URL)
		}
	}
	results := s.storeImages(ctx, game.ID, kinds, urls)
	if ctx.Err() != nil {
		var done []MediaAsset
		for _, r := range results {
			if r.err == nil {
				done = append(done, r.asset)
			}
		}
		s.store.removeFiles(done)
		return assetBatch{}, ctx.Err()
	}

	first := 0
	if hasCover {
		cover := results[0]
		first = 1
		if cover.err == nil {
			batch.assets = append(batch.assets, cover.asset)
			batch.created = append(batch.created, cover.asset)
			batch.cover = cover.asset.ID
		} else {
			slog.Warn("download cover", "game", game.ID, "url", meta.Cover.URL, "error", cover.err)
		}
	}
	if batch.cover == "" {
		if kept, ok := carryCover(previous, game.CoverAssetID); ok {
			batch.assets = append(batch.assets, kept)
			batch.cover = kept.ID
		}
	}

	if mode == modeArt {
		batch.assets = append(batch.assets, carried...)
		for i := first; i < len(results); i++ {
			if results[i].err != nil {
				slog.Warn("download hero image", "kind", kinds[i], "game", game.ID, "url", urls[i], "error", results[i].err)
				continue
			}
			batch.assets = append(batch.assets, results[i].asset)
			batch.created = append(batch.created, results[i].asset)
		}
		// A stored choice stands, including one made by an earlier rule.
		if kept, ok := keptHero(game.HeroAssetID, batch.assets); ok {
			batch.hero = kept.ID
		} else if wide, ok := wideHero(batch.assets); ok {
			batch.hero = wide.ID
		}
		// The gallery is still the full refresh's job: a game that has not had
		// one, or has no stored screenshots, stays partial, so opening its
		// page completes it. A hero screenshot alone is not a gallery.
		batch.partial = game.MetadataPartial || len(carryAssets(previous, AssetScreenshot)) == 0
		return batch, nil
	}

	fetched := map[AssetType][]MediaAsset{}
	listed := map[AssetType]bool{}
	for i := first; i < len(results); i++ {
		listed[kinds[i]] = true
		if results[i].err != nil {
			slog.Warn("download image", "kind", kinds[i], "game", game.ID, "url", urls[i], "error", results[i].err)
			continue
		}
		fetched[kinds[i]] = append(fetched[kinds[i]], results[i].asset)
		batch.created = append(batch.created, results[i].asset)
	}
	// Stored pictures stand in only for this answer's own pictures that all
	// failed to download: an answer that lists none has none, and pictures
	// of a previously matched game never stand in for the new one.
	same := sameMatch(game, meta)
	kept := func(kind AssetType) []MediaAsset {
		if len(fetched[kind]) > 0 || !listed[kind] || !same {
			return fetched[kind]
		}
		return carryAssets(previous, kind)
	}
	shots := kept(AssetScreenshot)
	arts := kept(AssetArtwork)
	batch.assets = append(batch.assets, shots...)
	batch.assets = append(batch.assets, arts...)
	if wide, ok := wideHero(batch.assets); ok {
		batch.hero = wide.ID
	}
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
		Resolved:    game.ExternalIDs.IGDB != "" || game.ExternalIDs.Steam != "",
		Stale:       s.stale(game),
	}
	if s.provider != nil {
		view.Provider = s.provider.Name()
	}
	hero := s.hero(game)
	view.Hero, view.HeroIsPoster = hero.asset.URL, hero.poster
	view.Match = s.matchState(game)
	view.StoreLinks = validStoreLinks(game.StoreLinks)
	view.Steam = s.steam.info(game.ID)
	return view
}

func (s *Service) needsSteam(game catalog.Game) bool {
	if _, ok := s.steam.get(game.ID); ok {
		return false
	}
	return s.steam.predates(game.MetadataUpdatedAt)
}

func (s *Service) matchState(game catalog.Game) MatchState {
	if s.provider == nil || game.ExternalIDs.IGDB != "" || game.ExternalIDs.Steam != "" {
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

func (s *Service) coverAsset(game catalog.Game) (MediaAsset, bool) {
	if asset, ok := s.store.find(game.CoverAssetID); ok && asset.GameID == game.ID {
		return asset, true
	}
	for _, a := range s.store.list(game.ID) {
		if a.Type == AssetCover {
			return a, true
		}
	}
	return MediaAsset{}, false
}

func (s *Service) coverURL(game catalog.Game) string {
	asset, ok := s.coverAsset(game)
	if !ok {
		return ""
	}
	return asset.URL
}

func (s *Service) stale(game catalog.Game) bool {
	lang := game.MetadataLanguage
	if lang == "" {
		lang = "en"
	}
	if lang != s.currentLanguage() || game.MetadataUpdatedAt == nil {
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
	ctx, cancel := context.WithTimeout(requestLanguage(base, s.currentLanguage()), timeout)
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

// The redesign's hero block is a full-width band: a picture narrower than
// minHeroWidth blurs across it, and one wider than maxHeroRatio is a panorama
// strip whose subject the band crops away.
const (
	minHeroWidth = 1280
	minHeroRatio = 1.5
	maxHeroRatio = 2.6
)

func fitsHero(width, height int) bool {
	if width < minHeroWidth || height <= 0 {
		return false
	}
	ratio := float64(width) / float64(height)
	return ratio >= minHeroRatio && ratio <= maxHeroRatio
}

// wideHero is the hero rule: the first screenshot that fits the band, else the
// first artwork that does, each in the order the provider listed them.
func wideHero(assets []MediaAsset) (MediaAsset, bool) {
	for _, kind := range []AssetType{AssetScreenshot, AssetArtwork} {
		for _, a := range assets {
			if a.Type == kind && fitsHero(a.Width, a.Height) {
				return a, true
			}
		}
	}
	return MediaAsset{}, false
}

// heroRef applies the hero rule to an answer's references, by the dimensions
// the provider reports, for a refresh that downloads only the hero.
func heroRef(shots, arts []ImageRef) (AssetType, ImageRef, bool) {
	for _, group := range []struct {
		kind AssetType
		refs []ImageRef
	}{{AssetScreenshot, shots}, {AssetArtwork, arts}} {
		for _, ref := range group.refs {
			if ref.URL != "" && fitsHero(ref.Width, ref.Height) {
				return group.kind, ref, true
			}
		}
	}
	return "", ImageRef{}, false
}

// hasHero reports whether the stored pictures already give the game a wide
// hero, either its stored choice or one that fits the rule.
func hasHero(game catalog.Game, assets []MediaAsset) bool {
	if _, ok := keptHero(game.HeroAssetID, assets); ok {
		return true
	}
	_, ok := wideHero(assets)
	return ok
}

func keptHero(id string, assets []MediaAsset) (MediaAsset, bool) {
	if id == "" {
		return MediaAsset{}, false
	}
	for _, a := range assets {
		if a.ID == id && (a.Type == AssetScreenshot || a.Type == AssetArtwork) {
			return a, true
		}
	}
	return MediaAsset{}, false
}

type heroPick struct {
	asset  MediaAsset
	poster bool
	ok     bool
}

// hero is the one answer to which picture heads a game, for the view, GetArt
// and ArtFiles. A full refresh persists wideHero's pick as HeroAssetID, so the
// stored ID is followed while it names a stored picture: a choice made by an
// earlier rule keeps showing until the next full refresh. Without a wide
// picture the cover stands in as a poster.
func (s *Service) hero(game catalog.Game) heroPick {
	assets := s.store.list(game.ID)
	if a, ok := keptHero(game.HeroAssetID, assets); ok {
		return heroPick{asset: a, ok: true}
	}
	if a, ok := wideHero(assets); ok {
		return heroPick{asset: a, ok: true}
	}
	if cover, ok := s.coverAsset(game); ok {
		return heroPick{asset: cover, poster: true, ok: true}
	}
	return heroPick{}
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

func carryAssets(previous []MediaAsset, kind AssetType) []MediaAsset {
	var out []MediaAsset
	for _, a := range previous {
		if a.Type == kind {
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

// SteamRequirements returns the stored Steam requirements of a catalog game;
// found is false while the store block has not been fetched.
//
//wails:ignore
func (s *Service) SteamRequirements(gameID string) (SteamRequirements, bool, error) {
	game, err := s.catalog.GetGame(strings.TrimSpace(gameID))
	if err != nil {
		return SteamRequirements{}, false, err
	}
	info := s.steam.info(game.ID)
	if info == nil {
		return SteamRequirements{}, false, nil
	}
	return info.Requirements, true, nil
}
