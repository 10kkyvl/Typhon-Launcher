package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventStarted   = "discovery:started"
	eventProgress  = "discovery:progress"
	eventCompleted = "discovery:completed"
)

var (
	ErrBusy       = errors.New("поиск игр уже выполняется")
	errNotStarted = errors.New("сервис поиска игр не запущен")
	errNoScan     = errors.New("поиск игр не выполняется")
)

type gameLibrary interface {
	GetInstalledGames() []library.Game
	ApplyDiscovered(d library.Discovered) (library.Game, library.Outcome, error)
}

type gameCatalog interface {
	Resolve(q catalog.Query) catalog.Match
	Provision(queries []catalog.Query) map[string]catalog.Game
}

type metadataResolver interface {
	EnsureFresh(gameID string) (bool, error)
}

type Service struct {
	mu       sync.Mutex
	settings *settings.Service
	library  gameLibrary
	catalog  gameCatalog
	metadata metadataResolver

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	running bool
	stop    context.CancelFunc
	wg      sync.WaitGroup
}

func NewService(settingsService *settings.Service, lib gameLibrary, cat gameCatalog, meta metadataResolver) (*Service, error) {
	if settingsService == nil {
		return nil, errors.New("настройки недоступны")
	}
	if lib == nil {
		return nil, errors.New("библиотека недоступна")
	}
	if cat == nil {
		return nil, errors.New("каталог игр недоступен")
	}
	return &Service{settings: settingsService, library: lib, catalog: cat, metadata: meta}, nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	stop := s.stop
	s.mu.Unlock()

	if stop != nil {
		stop()
	}
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Service) Scanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Service) CancelScan() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.stop == nil {
		return errNoScan
	}
	s.stop()
	return nil
}

func (s *Service) Scan() (Result, error) {
	ctx, err := s.begin()
	if err != nil {
		return Result{}, err
	}
	defer s.finish()
	return s.run(ctx)
}

func (s *Service) begin() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ctx == nil {
		return nil, errNotStarted
	}
	if s.running {
		return nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.running = true
	s.stop = cancel
	s.wg.Add(1)
	return ctx, nil
}

func (s *Service) finish() {
	s.mu.Lock()
	stop := s.stop
	s.running = false
	s.stop = nil
	s.mu.Unlock()

	if stop != nil {
		stop()
	}
	s.wg.Done()
}

func (s *Service) roots() ([]string, error) {
	cfg := s.settings.GetSettings()
	return normalizeRoots([]string{cfg.GamesPath})
}

func normalizeRoots(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		key, err := platform.PathKey(path)
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", path, err)
		}
		if seen[key] {
			continue
		}
		normalized, err := platform.Normalize(path)
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", path, err)
		}
		seen[key] = true
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil, settings.ErrLibraryNotConfigured
	}
	return out, nil
}

func (s *Service) resolveMetadata(ids []string, result *Result) {
	if s.metadata == nil || len(ids) == 0 {
		return
	}
	for _, id := range ids {
		if _, err := s.metadata.EnsureFresh(id); err != nil {
			slog.Warn("discovery metadata", "game", id, "error", err)
			result.note(id, fmt.Sprintf("метаданные не запрошены: %v", err))
		}
	}
}
