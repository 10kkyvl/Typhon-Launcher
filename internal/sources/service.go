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
	"strings"
	"sync"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/redact"
	"typhon/internal/settings"
	"typhon/internal/sources/feed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventUpdated        = "source:updated"
	eventError          = "source:error"
	eventReleaseAdded   = "release:added"
	eventReleaseRemoved = "release:removed"
	eventReleaseMatched = "release:matched"
	eventReleaseReview  = "release:needs-review"

	maxWarnings        = 10
	refreshConcurrency = 2
	refreshTimeout     = 3 * time.Minute
	scheduleTick       = time.Minute
	maxRetryDelay      = time.Hour
)

var (
	errSourceNotFound = errors.New("источник не найден")
	errSourceBusy     = errors.New("источник уже обновляется")
	errSourceDisabled = errors.New("источник отключён")
	errSourceExists   = errors.New("этот источник уже добавлен")
	errNoDialog       = errors.New("диалог выбора файла недоступен")
)

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
		stored, err := s.store.loadReleases(item.ID)
		if err != nil {
			return err
		}
		list := make([]*Release, 0, len(stored))
		for i := range stored {
			r := stored[i]
			list = append(list, &r)
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
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()
	s.wg.Add(1)
	go s.schedule()
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
	ctx, cancel := context.WithTimeout(s.context(), refreshTimeout)
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
	ctx, cancel := context.WithTimeout(s.context(), refreshTimeout)
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

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, src := range s.sources {
		if sameLocation(src, kind, location) || (src.Fingerprint != "" && src.Fingerprint == preview.Fingerprint) {
			preview.Duplicate = true
			break
		}
	}
	return preview
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
	ctx, cancel := context.WithTimeout(s.context(), refreshTimeout)
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

func (s *Service) context() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
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
	if kind == TypeFile {
		return feed.ReadFile(ctx, location)
	}
	return feed.Fetch(ctx, s.client, location, cond)
}

func (s *Service) fail(id string, err error, scheduled bool) {
	interval := refreshInterval(s.config())

	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return
	}
	src.Health = HealthError
	src.LastError = err.Error()
	src.Status = statusOf(src)
	s.failures[id]++
	s.retryAt[id] = time.Now().Add(retryDelay(s.failures[id], interval))
	if saveErr := s.store.saveSources(flatten(s.sources)); saveErr != nil {
		slog.Error("save sources", "error", saveErr)
	}
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
		slog.Error("save sources", "error", err)
	}

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

func (s *Service) notifyChanged() {
	s.mu.Lock()
	notify := s.onChanged
	s.mu.Unlock()
	if notify != nil {
		go notify()
	}
}

func (s *Service) schedule() {
	defer s.wg.Done()
	ticker := time.NewTicker(scheduleTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.context().Done():
			return
		case <-ticker.C:
			s.refreshDue()
		}
	}
}

func (s *Service) refreshDue() {
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
		case <-s.context().Done():
			return
		}
		ctx, cancel := context.WithTimeout(s.context(), refreshTimeout)
		if _, err := s.refresh(ctx, id, true); err != nil {
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
	if kind == TypeFile {
		return samePath(src.Path, location)
	}
	return strings.EqualFold(src.URL, location)
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
