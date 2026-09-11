package sources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/redact"
	"typhon/internal/settings"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventUpdated        = "source:updated"
	eventError          = "source:error"
	eventDegraded       = "source:degraded"
	eventReleaseAdded   = "release:added"
	eventReleaseRemoved = "release:removed"
	eventReleaseMatched = "release:matched"
	eventReleaseReview  = "release:needs-review"

	maxWarnings        = 10
	previewCacheTTL    = 10 * time.Minute
	refreshConcurrency = 2
	refreshTimeout     = 3 * time.Minute
	scheduleTick       = time.Minute
	maxRetryDelay      = time.Hour
)

var (
	errSourceNotFound = uierr.New("sources.source_not_found", "источник не найден")
	errSourceBusy     = uierr.New("sources.source_busy", "источник уже обновляется")
	errSourceDisabled = uierr.New("sources.source_disabled", "источник отключён")
	errSourceExists   = uierr.New("sources.source_exists", "этот источник уже добавлен")
	errNoDialog       = uierr.New("sources.dialog_unavailable", "диалог выбора файла недоступен")

	// errServiceNotStarted is a plain error, not a uierr code: it can only
	// happen if a caller reaches into the service before ServiceStartup runs
	// (invariant 20 forbids a context.Background() fallback instead), which
	// wails never does in production. It is deliberately left out of the
	// sources.* UI error table that TestErrorCodesMatchTheFrontendTable checks.
	errServiceNotStarted = errors.New("sources: service not started")
)

// degradedStatus is the source:degraded event payload. It stays unexported
// with no accessor method: adding either would change the wails bindings
// generated for Service, which is not allowed here.
type degradedStatus struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

type Service struct {
	mu       sync.Mutex
	store    *store
	catalog  *catalog.Service
	settings *settings.Service
	client   *http.Client

	sources  []*Source
	releases map[string][]*Release

	refreshing map[string]bool
	failures   map[string]int
	retryAt    map[string]time.Time
	sem        chan struct{}
	onChanged  func()
	status     degradedStatus
	cached     *cachedFeed

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewService(settingsService *settings.Service, cat *catalog.Service) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return newServiceAt(dir, settingsService, cat)
}

// NewServiceAt builds a source service whose state is isolated under dir.
// It is useful for integration checks and other hosts that provide their own
// configuration root instead of the desktop application's global one.
//
//wails:ignore
func NewServiceAt(dir string, cat *catalog.Service) (*Service, error) {
	return newServiceAt(dir, nil, cat)
}

func newServiceAt(dir string, settingsService *settings.Service, cat *catalog.Service) (*Service, error) {
	if dir == "" {
		return nil, errors.New("sources path unavailable")
	}
	s := &Service{
		store:      newStore(dir),
		catalog:    cat,
		settings:   settingsService,
		releases:   map[string][]*Release{},
		refreshing: map[string]bool{},
		failures:   map[string]int{},
		retryAt:    map[string]time.Time{},
		sem:        make(chan struct{}, refreshConcurrency),
		client:     feed.NewClient(),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) load() error {
	srcs, err := s.store.loadSources()
	if err != nil {
		return err
	}
	for _, src := range srcs {
		item := src
		if item.Type == "" {
			item.Type = TypeURL
		}
		s.sources = append(s.sources, &item)
		list, err := s.store.loadReleases(item.ID)
		if err != nil {
			return err
		}
		s.releases[item.ID] = list
	}
	if len(s.sources) > 0 {
		slog.Info("sources loaded", "sources", len(s.sources), "releases", s.totalReleases())
	}
	return nil
}

func (s *Service) totalReleases() int {
	total := 0
	for _, list := range s.releases {
		total += len(list)
	}
	return total
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	startupCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.ctx, s.cancel = startupCtx, cancel
	s.mu.Unlock()
	s.wg.Add(1)
	go s.scheduleLoop(startupCtx)
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

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// markDegradedLocked records a persist failure and notifies the frontend.
// The caller must hold s.mu.
func (s *Service) markDegradedLocked(err error) {
	s.status = degradedStatus{Degraded: true, Message: err.Error()}
	emit(eventDegraded, s.status)
}

// clearDegradedLocked resets a previously recorded persist failure once a
// save succeeds again. It only emits when the status actually changes, so a
// healthy service does not fire source:degraded on every successful save.
// The caller must hold s.mu.
func (s *Service) clearDegradedLocked() {
	if !s.status.Degraded {
		return
	}
	s.status = degradedStatus{}
	emit(eventDegraded, s.status)
}

// cachedFeed держит фид, который только что разобрали для превью, чтобы
// «Сохранить» не качало те же десятки мегабайт второй раз. Запись одна:
// превью — это шаг мастера добавления, а не фон.
type cachedFeed struct {
	kind     Type
	location string
	result   feed.Result
	at       time.Time
}

func (s *Service) rememberFeedLocked(kind Type, location string, result feed.Result) {
	s.cached = &cachedFeed{kind: kind, location: location, result: result, at: time.Now()}
}

// takeCachedFeed отдаёт разобранный фид ровно один раз: повторное обновление
// источника обязано сходить в сеть, иначе оно показывало бы старое содержимое.
func (s *Service) takeCachedFeed(kind Type, location string) (feed.Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cached := s.cached
	if cached == nil {
		return feed.Result{}, false
	}
	if cached.kind != kind || !sameLocationValue(kind, cached.location, location) {
		return feed.Result{}, false
	}
	s.cached = nil
	if time.Since(cached.at) > previewCacheTTL {
		return feed.Result{}, false
	}
	return cached.result, true
}

// pruneExpiredPreview drops the cached preview once it is older than
// previewCacheTTL, even if nothing ever calls takeCachedFeed again. It runs
// off scheduleLoop's existing once-a-minute ticker instead of a dedicated
// timer per preview: the cache holds at most one entry, so there is nothing
// to iterate, and reusing the ticker that is already there means TestSource
// never has to spin up a goroutine just to expire its own cache slot later.
func (s *Service) pruneExpiredPreview(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && now.Sub(s.cached.at) > previewCacheTTL {
		s.cached = nil
	}
}

func (s *Service) findLocked(id string) *Source {
	for _, src := range s.sources {
		if src.ID == id {
			return src
		}
	}
	return nil
}

func (s *Service) ListSources() []Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := flatten(s.sources)
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out
}

func (s *Service) GetSource(id string) (Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.findLocked(id)
	if src == nil {
		return Source{}, errSourceNotFound
	}
	return *src, nil
}

func (s *Service) GetSourceDetails(id string) (Details, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.findLocked(id)
	if src == nil {
		return Details{}, errSourceNotFound
	}
	details := Details{Source: *src}
	for _, r := range s.releases[id] {
		details.Total++
		switch {
		case r.Ignored:
			details.Ignored++
		case r.Availability == AvailabilityRemoved:
			details.Removed++
		default:
			details.Available++
		}
		if r.New {
			details.New++
		}
	}
	return details, nil
}

func (s *Service) TestSource(rawURL string) (Preview, error) {
	normalized, err := feed.ValidateURL(rawURL)
	if err != nil {
		return Preview{}, err
	}
	base, ok := s.context()
	if !ok {
		return Preview{}, errServiceNotStarted
	}
	ctx, cancel := context.WithTimeout(base, refreshTimeout)
	defer cancel()

	result, err := feed.Fetch(ctx, s.client, normalized, feed.Conditional{})
	if err != nil {
		slog.Warn("source test failed", "operation", "test", "host", redact.URL(normalized), "error", err)
		return Preview{}, err
	}
	return s.preview(TypeURL, normalized, result), nil
}

func (s *Service) TestSourceFile(rawPath string) (Preview, error) {
	path, err := feed.ValidatePath(rawPath)
	if err != nil {
		return Preview{}, err
	}
	base, ok := s.context()
	if !ok {
		return Preview{}, errServiceNotStarted
	}
	ctx, cancel := context.WithTimeout(base, refreshTimeout)
	defer cancel()

	result, err := feed.ReadFile(ctx, path)
	if err != nil {
		slog.Warn("source file test failed", "operation", "test", "error", err)
		return Preview{}, err
	}
	return s.preview(TypeFile, path, result), nil
}

func (s *Service) SelectFeedFile() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errNoDialog
	}
	path, err := app.Dialog.OpenFile().
		SetTitle("Выберите файл фида").
		CanChooseFiles(true).
		AddFilter("Файл фида (*.json)", "*.json").
		AddFilter("Все файлы", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("выбор файла фида: %w", err)
	}
	return path, nil
}

func (s *Service) preview(kind Type, location string, result feed.Result) Preview {
	preview := Preview{
		Name:        result.Feed.Name,
		Type:        kind,
		FeedVersion: result.Feed.Version,
		Entries:     len(result.Feed.Entries),
		Invalid:     result.Feed.Invalid,
		Warnings:    trimWarnings(result.Feed.Warnings),
		Fingerprint: result.Feed.Fingerprint,
	}
	if kind == TypeFile {
		preview.Path = location
	} else {
		preview.URL = location
	}
	if preview.Name == "" {
		preview.Name = displayName(kind, location)
	}
	if kind == TypeURL {
		preview.Insecure = insecureURL(location)
	}
	preview.Games, preview.Known = s.catalogCoverage(result.Feed.Entries)
	preview.Unknown = preview.Games - preview.Known

	s.mu.Lock()
	defer s.mu.Unlock()
	s.rememberFeedLocked(kind, location, result)
	for _, src := range s.sources {
		if sameLocation(src, kind, location) || (src.Fingerprint != "" && src.Fingerprint == preview.Fingerprint) {
			preview.Duplicate = true
			break
		}
	}
	return preview
}

// catalogCoverage считает, сколько разных игр в фиде и сколько из них уже есть
// в каталоге. Считается по уникальным нормализованным названиям: фид на 20
// тысяч записей обычно описывает вчетверо меньше игр, и «1200 игр, 340 у вас
// уже есть» — это ответ про игры, а не про строки.
func (s *Service) catalogCoverage(entries []feed.Entry) (games, known int) {
	if len(entries) == 0 {
		return 0, 0
	}
	queries := make([]catalog.Query, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		parsed := titles.Parse(e.Title)
		if e.Game != "" {
			parsed = titles.Parse(e.Game)
		}
		if parsed.Normalized == "" {
			continue
		}
		key := parsed.Normalized
		if parsed.Year > 0 {
			key += "|" + strconv.Itoa(parsed.Year)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		queries = append(queries, catalog.Query{Title: parsed.Base, Normalized: parsed.Normalized, Year: parsed.Year})
	}
	if len(queries) == 0 || s.catalog == nil {
		return len(queries), 0
	}
	for _, match := range s.catalog.ResolveAll(queries) {
		if match.Status == catalog.StatusMatched {
			known++
		}
	}
	return len(queries), known
}

func insecureURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "http://")
}

func (s *Service) AddSource(rawURL string) (Source, error) {
	normalized, err := feed.ValidateURL(rawURL)
	if err != nil {
		return Source{}, err
	}
	return s.addSource(TypeURL, normalized)
}

func (s *Service) AddSourceFile(rawPath string) (Source, error) {
	path, err := feed.ValidatePath(rawPath)
	if err != nil {
		return Source{}, err
	}
	return s.addSource(TypeFile, path)
}

func (s *Service) addSource(kind Type, location string) (Source, error) {
	s.mu.Lock()
	for _, src := range s.sources {
		if sameLocation(src, kind, location) {
			s.mu.Unlock()
			return Source{}, errSourceExists
		}
	}
	src := &Source{
		ID:        catalog.NewID(),
		Name:      displayName(kind, location),
		Type:      kind,
		Enabled:   true,
		Status:    StatusUpdating,
		Health:    HealthHealthy,
		CreatedAt: time.Now(),
	}
	if kind == TypeFile {
		src.Path = location
	} else {
		src.URL = location
		src.Insecure = insecureURL(location)
	}
	s.sources = append(s.sources, src)
	s.releases[src.ID] = nil
	if err := s.store.saveSources(flatten(s.sources)); err != nil {
		s.sources = s.sources[:len(s.sources)-1]
		delete(s.releases, src.ID)
		s.mu.Unlock()
		return Source{}, fmt.Errorf("save sources: %w", err)
	}
	id := src.ID
	snapshot := *src
	s.mu.Unlock()

	slog.Info("source added", "source_id", id, "type", string(kind))
	emit(eventUpdated, snapshot)

	if _, err := s.RefreshSource(id); err != nil {
		if current, getErr := s.GetSource(id); getErr == nil {
			return current, err
		}
		return snapshot, err
	}
	return s.GetSource(id)
}

func (s *Service) RemoveSource(id string) error {
	s.mu.Lock()
	for i, src := range s.sources {
		if src.ID != id {
			continue
		}
		name := src.Name
		s.sources = append(s.sources[:i], s.sources[i+1:]...)
		delete(s.releases, id)
		delete(s.failures, id)
		delete(s.retryAt, id)
		if err := s.store.saveSources(flatten(s.sources)); err != nil {
			s.mu.Unlock()
			return err
		}
		s.store.removeReleases(id)
		s.mu.Unlock()
		slog.Info("source removed", "source_id", id, "name", name)
		emit(eventUpdated, Source{ID: id})
		return nil
	}
	s.mu.Unlock()
	return errSourceNotFound
}

func (s *Service) SetSourceEnabled(id string, enabled bool) error {
	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return errSourceNotFound
	}
	src.Enabled = enabled
	src.Status = statusOf(src)
	if err := s.store.saveSources(flatten(s.sources)); err != nil {
		s.mu.Unlock()
		return err
	}
	snapshot := *src
	s.mu.Unlock()

	slog.Info("source enabled changed", "source_id", id, "enabled", enabled)
	emit(eventUpdated, snapshot)
	return nil
}

func (s *Service) RefreshSource(id string) (Summary, error) {
	base, ok := s.context()
	if !ok {
		return Summary{}, errServiceNotStarted
	}
	ctx, cancel := context.WithTimeout(base, refreshTimeout)
	defer cancel()
	return s.refresh(ctx, id, false)
}

func (s *Service) RefreshAll() []Summary {
	s.mu.Lock()
	ids := make([]string, 0, len(s.sources))
	for _, src := range s.sources {
		if src.Enabled {
			ids = append(ids, src.ID)
		}
	}
	s.mu.Unlock()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []Summary
	)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			s.sem <- struct{}{}
			defer func() { <-s.sem }()
			summary, err := s.RefreshSource(id)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, summary)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return results
}

// context returns the service's running context. It reports ok=false
// instead of substituting context.Background() when ServiceStartup has not
// run yet (invariant 20): callers refuse the operation in that case rather
// than run it under a context nothing will ever cancel.
func (s *Service) context() (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx, s.ctx != nil
}

func (s *Service) refresh(ctx context.Context, id string, scheduled bool) (Summary, error) {
	started := time.Now()

	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return Summary{}, errSourceNotFound
	}
	if s.refreshing[id] {
		s.mu.Unlock()
		return Summary{}, errSourceBusy
	}
	if scheduled && !src.Enabled {
		s.mu.Unlock()
		return Summary{}, errSourceDisabled
	}
	s.refreshing[id] = true
	src.Status = StatusUpdating
	initial := len(s.releases[id]) == 0
	cond := feed.Conditional{ETag: src.ETag, LastModified: src.LastModified}
	kind := src.Type
	location := locationOf(src)
	name := src.Name
	snapshot := *src
	s.mu.Unlock()

	emit(eventUpdated, snapshot)
	defer func() {
		s.mu.Lock()
		delete(s.refreshing, id)
		s.mu.Unlock()
	}()

	slog.Info("source refresh started", "source_id", id, "name", name)
	result, err := s.fetchFeed(ctx, kind, location, cond)
	if err != nil {
		s.fail(id, err, scheduled)
		return Summary{}, err
	}
	if result.NotModified {
		return s.settle(id, nil, result, started, initial, true)
	}
	return s.settle(id, parseEntries(id, result.Feed.Entries, time.Now()), result, started, initial, false)
}

func (s *Service) fetchFeed(ctx context.Context, kind Type, location string, cond feed.Conditional) (feed.Result, error) {
	if result, ok := s.takeCachedFeed(kind, location); ok {
		slog.Info("source feed served from the preview cache", "type", string(kind))
		return result, nil
	}
	if kind == TypeFile {
		return feed.ReadFile(ctx, location)
	}
	return feed.Fetch(ctx, s.client, location, cond)
}

// fail records that a refresh failed. A persist failure here rolls the
// in-memory source and backoff bookkeeping back and marks the service
// degraded instead of returning: fail's own callers already have a more
// specific error to return (the fetch/merge failure that led here), and
// replacing that with a persistence error would hide the actual cause.
func (s *Service) fail(id string, err error, scheduled bool) {
	interval := refreshInterval(s.config())

	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return
	}
	before := *src
	beforeFailures, hadFailures := s.failures[id]
	beforeRetry, hadRetry := s.retryAt[id]
	src.Health = HealthError
	src.LastError = err.Error()
	src.Status = statusOf(src)
	s.failures[id]++
	s.retryAt[id] = time.Now().Add(retryDelay(s.failures[id], interval))
	if saveErr := s.store.saveSources(flatten(s.sources)); saveErr != nil {
		*src = before
		if hadFailures {
			s.failures[id] = beforeFailures
		} else {
			delete(s.failures, id)
		}
		if hadRetry {
			s.retryAt[id] = beforeRetry
		} else {
			delete(s.retryAt, id)
		}
		s.markDegradedLocked(saveErr)
		s.mu.Unlock()
		slog.Error("persist source failure state", "source_id", id, "error", saveErr)
		return
	}
	s.clearDegradedLocked()
	snapshot := *src
	s.mu.Unlock()

	slog.Error("source refresh failed", "source_id", id, "name", snapshot.Name, "error", err)
	emit(eventUpdated, snapshot)
	emit(eventError, SourceError{SourceID: id, Name: snapshot.Name, Message: err.Error(), Scheduled: scheduled})
}

func retryDelay(failures int, interval time.Duration) time.Duration {
	delay := scheduleTick
	for i := 1; i < failures && delay < maxRetryDelay; i++ {
		delay *= 2
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	if interval > 0 && delay > interval {
		delay = interval
	}
	return delay
}

func (s *Service) settle(id string, incoming []*Release, result feed.Result, started time.Time, initial, notModified bool) (Summary, error) {
	now := time.Now()

	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return Summary{}, errSourceNotFound
	}
	beforeSrc := *src
	beforeFailures, hadFailures := s.failures[id]
	beforeRetry, hadRetry := s.retryAt[id]
	summary := Summary{SourceID: id, NotModified: notModified}
	if !notModified {
		previous := s.releases[id]
		list, mergeSummary := merge(previous, incoming, now, initial)
		mergeSummary.SourceID = id
		summary = mergeSummary
		if err := applyMatches(s.catalog, list); err != nil {
			s.mu.Unlock()
			s.fail(id, err, false)
			return Summary{SourceID: id, Error: err.Error()}, err
		}
		s.releases[id] = list
		if err := s.store.saveReleases(id, list); err != nil {
			s.releases[id] = previous
			s.mu.Unlock()
			s.fail(id, err, false)
			return Summary{SourceID: id, Error: err.Error()}, err
		}
		if result.Feed.Name != "" {
			src.Name = result.Feed.Name
		} else if src.Name == "" {
			src.Name = displayName(src.Type, locationOf(src))
		}
		src.FeedVersion = result.Feed.Version
		src.Fingerprint = result.Feed.Fingerprint
		src.Invalid = result.Feed.Invalid
		src.Warnings = trimWarnings(result.Feed.Warnings)
	}

	matched, review, unmatched := counts(s.releases[id])
	src.Entries = len(s.releases[id])
	src.Matched = matched
	src.Review = review
	src.Unmatched = unmatched
	src.LastError = ""
	src.LastUpdatedAt = &now
	delete(s.failures, id)
	delete(s.retryAt, id)
	if result.ETag != "" {
		src.ETag = result.ETag
	}
	if result.LastModified != "" {
		src.LastModified = result.LastModified
	}
	if src.Invalid > 0 || len(src.Warnings) > 0 {
		src.Health = HealthWarning
	} else {
		src.Health = HealthHealthy
	}
	src.Status = statusOf(src)
	if err := s.store.saveSources(flatten(s.sources)); err != nil {
		// The releases for this source (if any were merged above) are
		// already committed to disk; only the Source metadata computed from
		// them rolls back here, so the two never disagree (invariant I.4).
		*src = beforeSrc
		if hadFailures {
			s.failures[id] = beforeFailures
		} else {
			delete(s.failures, id)
		}
		if hadRetry {
			s.retryAt[id] = beforeRetry
		} else {
			delete(s.retryAt, id)
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return Summary{SourceID: id, Error: err.Error()}, fmt.Errorf("save sources: %w", err)
	}
	s.clearDegradedLocked()

	summary.Name = src.Name
	summary.Entries = src.Entries
	summary.Invalid = src.Invalid
	summary.Matched = matched
	summary.Review = review
	summary.Unmatched = unmatched
	summary.DurationMs = time.Since(started).Milliseconds()
	snapshot := *src
	s.mu.Unlock()

	slog.Info("source refreshed",
		"source_id", id,
		"name", snapshot.Name,
		"entries", summary.Entries,
		"invalid", summary.Invalid,
		"added", summary.Added,
		"removed", summary.Removed,
		"matched", summary.Matched,
		"review", summary.Review,
		"unmatched", summary.Unmatched,
		"new", summary.New,
		"notModified", notModified,
		"ms", summary.DurationMs)

	emit(eventUpdated, snapshot)
	if summary.Added > 0 {
		emit(eventReleaseAdded, ReleaseBatch{SourceID: id, Count: summary.Added})
	}
	if summary.Removed > 0 {
		emit(eventReleaseRemoved, ReleaseBatch{SourceID: id, Count: summary.Removed})
	}
	if summary.Matched > 0 {
		emit(eventReleaseMatched, ReleaseBatch{SourceID: id, Count: summary.Matched})
	}
	if summary.Review > 0 {
		emit(eventReleaseReview, ReleaseBatch{SourceID: id, Count: summary.Review})
	}
	if summary.Added > 0 || summary.Updated > 0 || summary.Restored > 0 {
		s.notifyChanged()
	}
	return summary, nil
}

//wails:ignore
func (s *Service) SetOnChanged(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChanged = fn
}

// notifyChanged runs the onChanged callback in its own goroutine so a slow
// listener (updates.HandleSourcesRefreshed walks every installed game)
// cannot block settle. The goroutine is counted in s.wg and refuses to start
// once the service is closing, so ServiceShutdown's wg.Wait() cannot return
// while one is still in flight (invariant 19).
func (s *Service) notifyChanged() {
	s.mu.Lock()
	notify := s.onChanged
	if notify == nil || s.closing {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		notify()
	}()
}

// schedule stays for tests that exercise the no-op-without-started-service
// invariant (TestScheduleAndRefreshDueNoOpWithoutStartedService): it still
// reads s.ctx via s.context() and refuses to run before ServiceStartup, the
// same way the goroutine used to. ServiceStartup itself now launches
// scheduleLoop directly with the ctx it just derived, so the real goroutine
// chain threads ctx as an explicit parameter (invariant 21, contextcheck)
// instead of re-fetching it from the receiver under s.mu.
func (s *Service) schedule() {
	ctx, ok := s.context()
	if !ok {
		s.wg.Done()
		return
	}
	s.scheduleLoop(ctx)
}

func (s *Service) scheduleLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(scheduleTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pruneExpiredPreview(time.Now())
			s.runRefreshDue(ctx)
		}
	}
}

// refreshDue mirrors schedule above: kept for tests that call it directly on
// a service whose ctx was set without going through ServiceStartup, still
// refusing via s.context(). scheduleLoop calls runRefreshDue with its own
// ctx directly instead of going through this fetch-from-receiver path.
func (s *Service) refreshDue() {
	ctx, ok := s.context()
	if !ok {
		return
	}
	s.runRefreshDue(ctx)
}

func (s *Service) runRefreshDue(ctx context.Context) {
	interval := refreshInterval(s.config())
	if interval <= 0 {
		return
	}
	now := time.Now()

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	var due []string
	for _, src := range s.sources {
		if !src.Enabled || s.refreshing[src.ID] {
			continue
		}
		if next, ok := s.retryAt[src.ID]; ok && now.Before(next) {
			continue
		}
		if src.LastUpdatedAt == nil || now.Sub(*src.LastUpdatedAt) >= interval {
			due = append(due, src.ID)
		}
	}
	s.mu.Unlock()

	for _, id := range due {
		select {
		case s.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		refreshCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
		if _, err := s.refresh(refreshCtx, id, true); err != nil {
			slog.Warn("scheduled refresh failed", "source_id", id, "error", err)
		}
		cancel()
		<-s.sem
	}
}

func (s *Service) config() settings.Settings {
	if s.settings == nil {
		return settings.Defaults()
	}
	return s.settings.GetSettings()
}

func refreshInterval(cfg settings.Settings) time.Duration {
	switch cfg.SourceRefreshInterval {
	case settings.RefreshManual:
		return 0
	case settings.RefreshHourly:
		return time.Hour
	case settings.RefreshHalfDay:
		return 12 * time.Hour
	case settings.RefreshDaily:
		return 24 * time.Hour
	default:
		return 6 * time.Hour
	}
}

func statusOf(src *Source) Status {
	switch {
	case !src.Enabled:
		return StatusDisabled
	case src.LastError != "":
		return StatusError
	default:
		return StatusActive
	}
}

func flatten(list []*Source) []Source {
	out := make([]Source, 0, len(list))
	for _, src := range list {
		out = append(out, *src)
	}
	return out
}

func trimWarnings(warnings []string) []string {
	if len(warnings) > maxWarnings {
		return append([]string(nil), warnings[:maxWarnings]...)
	}
	return warnings
}

func locationOf(src *Source) string {
	if src.Type == TypeFile {
		return src.Path
	}
	return src.URL
}

func sameLocation(src *Source, kind Type, location string) bool {
	if src.Type != kind {
		return false
	}
	return sameLocationValue(kind, locationOf(src), location)
}

func sameLocationValue(kind Type, a, b string) bool {
	if kind == TypeFile {
		return samePath(a, b)
	}
	return strings.EqualFold(a, b)
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func displayName(kind Type, location string) string {
	if kind == TypeFile {
		return fileName(location)
	}
	return hostName(location)
}

func fileName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "Источник"
	}
	return name
}

func hostName(rawURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	if idx := strings.IndexAny(trimmed, "/?#"); idx > 0 {
		trimmed = trimmed[:idx]
	}
	if trimmed == "" {
		return "Источник"
	}
	return trimmed
}
