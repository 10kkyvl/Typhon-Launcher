package redist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/uierr"
)

type Service struct {
	games func(id string) (library.Game, error)

	mu   sync.Mutex
	ctx  context.Context
	busy bool
}

func NewService(games func(id string) (library.Game, error)) (*Service, error) {
	if games == nil {
		return nil, errors.New("redist: не задан поиск игры")
	}
	return &Service{games: games}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	return nil
}

func (s *Service) context() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		return nil, uierr.New("redist.failed", "сервис ещё не запущен")
	}
	return s.ctx, nil
}

func (s *Service) installDir(gameID string) (string, error) {
	game, err := s.games(gameID)
	if err != nil {
		return "", uierr.Wrap("redist.failed", err)
	}
	if game.InstallDir == "" {
		return "", uierr.New("redist.no_install_dir", "у игры не задана папка установки")
	}
	return game.InstallDir, nil
}

func (s *Service) Scan(gameID string) ([]Package, error) {
	ctx, err := s.context()
	if err != nil {
		return nil, err
	}
	dir, err := s.installDir(gameID)
	if err != nil {
		return nil, err
	}
	pkgs, err := Scan(ctx, dir)
	if err != nil {
		return nil, uierr.Wrap("redist.failed", err)
	}
	return pkgs, nil
}

func (s *Service) InstalledRuntimes() ([]Runtime, error) {
	ctx, err := s.context()
	if err != nil {
		return nil, err
	}
	runtimes, err := InstalledVC(ctx)
	if errors.Is(err, ErrUnsupported) {
		return nil, uierr.Wrap("redist.unsupported", err)
	}
	if err != nil {
		return nil, uierr.Wrap("redist.failed", err)
	}
	return runtimes, nil
}

func (s *Service) Install(gameID, path string) (Result, error) {
	ctx, err := s.context()
	if err != nil {
		return Result{}, err
	}
	dir, err := s.installDir(gameID)
	if err != nil {
		return Result{}, err
	}
	if !platform.Inside(dir, path) {
		return Result{}, uierr.New("redist.outside_install", "установщик лежит вне папки игры")
	}
	if err := CheckInside(dir, path); err != nil {
		if errors.Is(err, ErrOutsideInstall) || errors.Is(err, ErrLinkInPath) || errors.Is(err, ErrRootNotDir) {
			return Result{}, uierr.Wrap("redist.outside_install", err)
		}
		return Result{}, uierr.Wrap("redist.failed", err)
	}
	pkg, ok := Recognize(dir, path)
	if !ok {
		return Result{}, uierr.Wrap("redist.outside_install", ErrNotRedist)
	}

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return Result{}, uierr.New("redist.busy", "другой компонент уже устанавливается")
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	result, err := Install(ctx, pkg)
	switch {
	case errors.Is(err, ErrElevationDeclined):
		return Result{}, uierr.Wrap("redist.elevation_declined", err)
	case errors.Is(err, ErrUnsupported):
		return Result{}, uierr.Wrap("redist.unsupported", err)
	case errors.Is(err, ErrCancelFailed):
		return Result{}, uierr.Wrap("redist.cancel_failed", err)
	case err != nil:
		slog.Warn("redist install failed", "game", gameID, "installer", pkg.Name, "error", err)
		return Result{}, uierr.Wrap("redist.failed", fmt.Errorf("установка %s: %w", pkg.Name, err))
	}
	return result, nil
}

func (s *Service) OfficialLinks() []Link {
	return OfficialLinks()
}
