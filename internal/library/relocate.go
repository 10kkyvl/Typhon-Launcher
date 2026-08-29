package library

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"typhon/internal/platform"
)

var (
	errRelocateEmptyInstallDir    = errors.New("не указан новый каталог установки")
	errRelocateRelativeInstallDir = errors.New("новый каталог установки должен быть абсолютным путём")
	ErrExecutableOutside          = errors.New("исполняемый файл лежит вне каталога установки")
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
		if oldInstallDir == "" || !platform.Inside(oldInstallDir, previous.Executable) {
			s.mu.Unlock()
			return Game{}, fmt.Errorf("%s: %w", previous.Executable, ErrExecutableOutside)
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
	s.mu.Unlock()

	if err := WriteMarker(newInstallDir, markerFor(next)); err != nil {
		slog.Error("write install marker after relocate", "id", id, "error", err)
	}
	if hasShortcut {
		if err := s.CreateShortcut(id); err != nil {
			return next, err
		}
		if updated, err := s.Find(id); err == nil {
			return updated, nil
		}
	}
	return next, nil
}
