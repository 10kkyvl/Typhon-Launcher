package sources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"typhon/internal/catalog"
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
)

var (
	errSourceNotFound = errors.New("источник не найден")
	errSourceBusy     = errors.New("источник уже обновляется")
	errSourceDisabled = errors.New("источник отключён")
	errSourceExists   = errors.New("этот источник уже добавлен")
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
	sem        chan struct{}

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewService(settingsService *settings.Service, cat *catalog.Service) *Service {
	dir, err := settings.ConfigDir()
	if err != nil {
		slog.Error("resolve config dir", "error", err)
		dir = ""
	}
	return newServiceAt(dir, settingsService, cat)
}

func newServiceAt(dir string, settingsService *settings.Service, cat *catalog.Service) *Service {
	s := &Service{
		store:      newStore(dir),
		catalog:    cat,
		settings:   settingsService,
		releases:   map[string][]*Release{},
		refreshing: map[string]bool{},
		sem:        make(chan struct{}, refreshConcurrency),
		client:     &http.Client{Timeout: feed.FetchTimeout},
	}
	s.load()
	return s
}

func (s *Service) load() {
	for _, src := range s.store.loadSources() {
		item := src
		s.sources = append(s.sources, &item)
		stored := s.store.loadReleases(item.ID)
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
		slog.Warn("source test failed", "url", normalized, "error", err)
		return Preview{}, err
	}
	preview := Preview{
		Name:        result.Feed.Name,
		URL:         normalized,
		FeedVersion: result.Feed.Version,
		Entries:     len(result.Feed.Entries),
		Invalid:     result.Feed.Invalid,
		Warnings:    trimWarnings(result.Feed.Warnings),
		Fingerprint: result.Feed.Fingerprint,
	}
	if preview.Name == "" {
		preview.Name = hostName(normalized)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, src := range s.sources {
		if strings.EqualFold(src.URL, normalized) || (src.Fingerprint != "" && src.Fingerprint == preview.Fingerprint) {
			preview.Duplicate = true
			break
		}
	}
	return preview, nil
}

func (s *Service) AddSource(rawURL string) (Source, error) {
	normalized, err := feed.ValidateURL(rawURL)
	if err != nil {
		return Source{}, err
	}

	s.mu.Lock()
	for _, src := range s.sources {
		if strings.EqualFold(src.URL, normalized) {
			s.mu.Unlock()
			return Source{}, errSourceExists
		}
	}
	src := &Source{
		ID:        catalog.NewID(),
		Name:      hostName(normalized),
		URL:       normalized,
		Enabled:   true,
		Status:    StatusUpdating,
		Health:    HealthHealthy,
		CreatedAt: time.Now(),
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

	slog.Info("source added", "id", id, "url", normalized)
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
		if err := s.store.saveSources(flatten(s.sources)); err != nil {
			s.mu.Unlock()
			return err
		}
		s.store.removeReleases(id)
		s.mu.Unlock()
		slog.Info("source removed", "id", id, "name", name)
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

	slog.Info("source enabled changed", "id", id, "enabled", enabled)
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
	url := src.URL
	name := src.Name
	snapshot := *src
	s.mu.Unlock()

	emit(eventUpdated, snapshot)
	defer func() {
		s.mu.Lock()
		delete(s.refreshing, id)
		s.mu.Unlock()
	}()

	slog.Info("source refresh started", "id", id, "name", name)
	result, err := feed.Fetch(ctx, s.client, url, cond)
	if err != nil {
		s.fail(id, err)
		return Summary{}, err
	}
	if result.NotModified {
		return s.settle(id, nil, result, started, initial, true), nil
	}
	return s.settle(id, parseEntries(id, result.Feed.Entries, time.Now()), result, started, initial, false), nil
}

func (s *Service) fail(id string, err error) {
	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return
	}
	src.Health = HealthError
	src.LastError = err.Error()
	src.Status = statusOf(src)
	if saveErr := s.store.saveSources(flatten(s.sources)); saveErr != nil {
		slog.Error("save sources", "error", saveErr)
	}
	snapshot := *src
	s.mu.Unlock()

	slog.Error("source refresh failed", "id", id, "name", snapshot.Name, "error", err)
	emit(eventUpdated, snapshot)
	emit(eventError, SourceError{SourceID: id, Name: snapshot.Name, Message: err.Error()})
}

func (s *Service) settle(id string, incoming []*Release, result feed.Result, started time.Time, initial, notModified bool) Summary {
	now := time.Now()

	s.mu.Lock()
	src := s.findLocked(id)
	if src == nil {
		s.mu.Unlock()
		return Summary{}
	}
	summary := Summary{SourceID: id, NotModified: notModified}
	if !notModified {
		list, mergeSummary := merge(s.releases[id], incoming, now, initial)
		mergeSummary.SourceID = id
		summary = mergeSummary
		applyMatches(s.catalog, list)
		s.releases[id] = list
		if err := s.store.saveReleases(id, list); err != nil {
			slog.Error("save releases", "source", id, "error", err)
		}
		if result.Feed.Name != "" {
			src.Name = result.Feed.Name
		} else if src.Name == "" {
			src.Name = hostName(src.URL)
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
		"id", id,
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
	return summary
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
			slog.Warn("scheduled refresh failed", "source", id, "error", err)
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
