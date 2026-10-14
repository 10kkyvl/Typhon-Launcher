package catalog

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/storage"
)

const (
	catalogPageStaleWindow = 30 * time.Minute
	pageEventName          = "catalog:page"
)

// PageEvent carries the result of a background refresh of a first page that
// BrowseGames answered from the local page cache with Stale set.
type PageEvent struct {
	Key  string   `json:"key"`
	Page GamePage `json:"page"`
}

type pageLife struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	refreshing map[string]struct{}
}

func emitEvent(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	own, cancel := context.WithCancel(ctx)
	s.life.mu.Lock()
	s.life.ctx, s.life.cancel = own, cancel
	s.life.refreshing = map[string]struct{}{}
	s.life.mu.Unlock()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.life.mu.Lock()
	cancel := s.life.cancel
	s.life.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.life.wg.Wait()
	return nil
}

func (s *Service) lifetimeAlive() bool {
	s.life.mu.Lock()
	defer s.life.mu.Unlock()
	return s.life.ctx != nil && s.life.ctx.Err() == nil
}

// startRefresh runs fn in the service's lifetime unless a refresh for the key
// is already in flight. It reports false when the service is not running, so
// the caller falls back to the synchronous path. wg.Add happens under the same
// lock that ServiceShutdown's cancel races against: once the context is done no
// new goroutine can be registered after Wait began.
func (s *Service) startRefresh(key string, fn func(ctx context.Context)) bool {
	s.life.mu.Lock()
	defer s.life.mu.Unlock()
	if s.life.ctx == nil || s.life.ctx.Err() != nil {
		return false
	}
	if _, busy := s.life.refreshing[key]; busy {
		return true
	}
	s.life.refreshing[key] = struct{}{}
	ctx := s.life.ctx
	s.life.wg.Add(1)
	go func() {
		defer s.life.wg.Done()
		defer func() {
			s.life.mu.Lock()
			delete(s.life.refreshing, key)
			s.life.mu.Unlock()
		}()
		fn(ctx)
	}()
	return true
}

// staleFirstPage answers an initial first-page request from the page cache and
// refreshes it in the background. Only the page the frontend asks for with no
// revision can be stale: continuations and prefix reloads pin a revision and
// must come from the backend.
func (s *Service) staleFirstPage(q GameQuery, durable bool, remote RemoteCatalog, path, key, snapshot string) (GamePage, bool) {
	if !durable || remote == nil || q.Page > 1 || q.Revision != 0 || !s.lifetimeAlive() {
		return GamePage{}, false
	}
	var page GamePage
	if err := storage.Load(path, 1, nil, &page); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("catalog page cache unreadable; fetching", "key", key, "error", err)
		}
		return GamePage{}, false
	}
	age := s.clock().Sub(page.CachedAt)
	if age < 0 || age >= catalogPageStaleWindow {
		return GamePage{}, false
	}
	page.Offline = false
	page.Stale = true
	page.Key = key
	s.normalizeCachedPage(&page)
	page.Snapshot = snapshot
	if page.PersonalizationFallback {
		s.mu.Lock()
		if snap, ok := s.browseSnapshots[snapshot]; ok {
			snap.query.Sort, snap.query.Profile = "popular", ""
			s.browseSnapshots[snapshot] = snap
		}
		s.mu.Unlock()
	}
	page.PersonalizationFallback = page.PersonalizationFallback || s.recommendationLoadErr != nil
	if !s.startRefresh(key, func(ctx context.Context) {
		s.refreshPage(ctx, q, remote, path, key, snapshot)
	}) {
		return GamePage{}, false
	}
	return page, true
}

func (s *Service) refreshPage(ctx context.Context, q GameQuery, remote RemoteCatalog, path, key, snapshot string) {
	var timing browseTiming
	page, err := s.fetchRemotePage(ctx, q, remote, snapshot, &timing)
	if err != nil {
		slog.Debug("catalog page refresh", "key", key, "error", err)
		return
	}
	page, err = s.commitPage(page, path, key, snapshot, &timing)
	if err != nil {
		slog.Warn("catalog page refresh", "key", key, "error", err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	s.emit(pageEventName, PageEvent{Key: key, Page: page})
}
