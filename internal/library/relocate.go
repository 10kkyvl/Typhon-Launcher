package library

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	errRelocateEmptyInstallDir    = errors.New("не указан новый каталог установки")
	errRelocateRelativeInstallDir = errors.New("новый каталог установки должен быть абсолютным путём")
	ErrExecutableOutside          = uierr.New("library.executable_outside", "исполняемый файл лежит вне каталога установки")
	ErrGameRelocating             = uierr.New("library.relocating", "игра переносится на другой диск, запуск недоступен")
	ErrRelocateGameRunning        = errors.New("игра запущена")
)

// Relocate rebases a game's on-disk pointers onto a new InstallDir after
// internal/relocate has already moved its files there. It never moves data
// itself.
//
//wails:ignore
func (s *Service) Relocate(id, newInstallDir string) (Game, error) {
	newInstallDir = strings.TrimSpace(newInstallDir)
	if newInstallDir == "" {
		return Game{}, errRelocateEmptyInstallDir
	}
	if !filepath.IsAbs(newInstallDir) {
		return Game{}, fmt.Errorf("%w: %s", errRelocateRelativeInstallDir, newInstallDir)
	}
	newInstallDir = filepath.Clean(newInstallDir)

	s.mu.Lock()
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return Game{}, errNotFound
	}
	previous := *game
	oldInstallDir := previous.InstallDir

	next := previous
	if previous.Executable != "" {
		if err := checkExecutableInside(previous); err != nil {
			s.mu.Unlock()
			return Game{}, err
		}
		rel, err := filepath.Rel(oldInstallDir, previous.Executable)
		if err != nil {
			s.mu.Unlock()
			return Game{}, fmt.Errorf("rebase executable: %w", err)
		}
		next.Executable = filepath.Join(newInstallDir, rel)
	}
	if previous.SavesDir != "" && oldInstallDir != "" && platform.Inside(oldInstallDir, previous.SavesDir) {
		rel, err := filepath.Rel(oldInstallDir, previous.SavesDir)
		if err != nil {
			s.mu.Unlock()
			return Game{}, fmt.Errorf("rebase saves dir: %w", err)
		}
		next.SavesDir = filepath.Join(newInstallDir, rel)
	}
	if previous.Cover != "" && oldInstallDir != "" && platform.Inside(oldInstallDir, previous.Cover) {
		rel, err := filepath.Rel(oldInstallDir, previous.Cover)
		if err != nil {
			s.mu.Unlock()
			return Game{}, fmt.Errorf("rebase cover: %w", err)
		}
		next.Cover = filepath.Join(newInstallDir, rel)
	}
	next.Uninstall.Command = rebaseUninstallCommand(next.Uninstall.Command, oldInstallDir, newInstallDir)
	next.Uninstall.QuietCommand = rebaseUninstallCommand(next.Uninstall.QuietCommand, oldInstallDir, newInstallDir)
	next.InstallDir = newInstallDir

	*game = next
	if err := s.persist(); err != nil {
		*game = previous
		s.mu.Unlock()
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("game relocated", "id", id, "title", next.Title, "installDir", newInstallDir)
	s.emitUpdated()
	hasShortcut := next.ShortcutPath != ""
	ctx := s.ctx
	s.mu.Unlock()

	markerErr := WriteMarker(newInstallDir, markerFor(next))
	if markerErr != nil {
		markerErr = fmt.Errorf("write install marker after relocate: %w", markerErr)
	}
	// Игра уже переехала и сохранена: сбой Steam не повод отменять перенос, запись
	// в Steam подтянется при следующей сверке.
	s.reconcileSteam(ctx, "relocate")
	if hasShortcut {
		if err := s.CreateShortcut(id); err != nil {
			return next, errors.Join(markerErr, err)
		}
		if markerErr == nil {
			if updated, err := s.Find(id); err == nil {
				return updated, nil
			}
		}
	}
	return next, markerErr
}

func checkExecutableInside(game Game) error {
	if game.Executable == "" {
		return nil
	}
	if game.InstallDir == "" || !platform.Inside(game.InstallDir, game.Executable) {
		return fmt.Errorf("%s: %w", game.Executable, ErrExecutableOutside)
	}
	return nil
}

// CheckRelocatable lets internal/relocate refuse a game that Relocate would
// reject before the first byte of its files is moved.
//
//wails:ignore
func (s *Service) CheckRelocatable(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	return checkExecutableInside(*game)
}

// BeginRelocate reserves the game for a move: it fails while the game runs or
// starts, and PlayGame refuses the game until release is called. The check and
// the reservation share one critical section, so a launch cannot slip in
// between them.
//
//wails:ignore
func (s *Service) BeginRelocate(id string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.findLocked(id) == nil {
		return nil, errNotFound
	}
	if _, ok := s.relocating[id]; ok {
		return nil, ErrGameRelocating
	}
	if _, ok := s.running[id]; ok || s.starting[id] != nil {
		return nil, ErrRelocateGameRunning
	}
	if s.relocating == nil {
		s.relocating = map[string]struct{}{}
	}
	s.relocating[id] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.relocating, id)
			s.mu.Unlock()
		})
	}, nil
}

func rebaseUninstallCommand(command, oldRoot, newRoot string) string {
	if oldRoot == "" {
		return command
	}
	// Require a separator after the old directory to avoid matching sibling names.
	for _, sep := range []string{"/", `\`} {
		old := strings.TrimRight(oldRoot, `/\`) + sep
		for start := 0; start < len(command); {
			index := strings.Index(strings.ToLower(command[start:]), strings.ToLower(old))
			if index < 0 {
				break
			}
			index += start
			if index == 0 || strings.ContainsRune("\"' =", rune(command[index-1])) {
				command = command[:index] + strings.TrimRight(newRoot, `/\`) + sep + command[index+len(old):]
				start = index + len(newRoot) + 1
			} else {
				start = index + len(old)
			}
		}
	}
	return command
}
