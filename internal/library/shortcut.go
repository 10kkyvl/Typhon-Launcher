package library

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/shortcut"
)

var (
	errShortcutUnsupported  = errors.New("ярлыки поддерживаются только в Windows")
	errShortcutUninstalled  = errors.New("игра не установлена")
	errShortcutNoExecutable = errors.New("у игры не задан исполняемый файл")
	errShortcutBadID        = errors.New("идентификатор игры непригоден для командной строки")
)

type shortcutBackend interface {
	Supported() bool
	DesktopDir() (string, error)
	FileName(title string) (string, error)
	Create(path string, link shortcut.Link) error
	Remove(path string) error
}

type systemShortcuts struct{}

func (systemShortcuts) Supported() bool                        { return shortcut.Supported() }
func (systemShortcuts) DesktopDir() (string, error)            { return shortcut.DesktopDir() }
func (systemShortcuts) FileName(title string) (string, error)  { return shortcut.FileName(title) }
func (systemShortcuts) Create(p string, l shortcut.Link) error { return shortcut.Create(p, l) }
func (systemShortcuts) Remove(p string) error                  { return shortcut.Remove(p) }

func (s *Service) CreateShortcut(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.shortcuts.Supported() {
		return errShortcutUnsupported
	}
	game := s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	if game.Uninstalled {
		return errShortcutUninstalled
	}
	if game.Executable == "" {
		return errShortcutNoExecutable
	}
	if !safeCommandLineID(id) {
		return fmt.Errorf("%w: %q", errShortcutBadID, id)
	}
	if _, err := os.Stat(game.Executable); err != nil {
		return fmt.Errorf("исполняемый файл игры: %w", err)
	}
	launcher, err := s.launcherPath()
	if err != nil {
		return fmt.Errorf("путь к лаунчеру: %w", err)
	}
	dir, err := s.shortcuts.DesktopDir()
	if err != nil {
		return err
	}
	name, err := s.shortcuts.FileName(game.Title)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name)
	// Иконку Windows берёт прямо из исполняемого файла игры, поэтому
	// отдельный .ico не нужен.
	link := shortcut.Link{
		Target:      launcher,
		Args:        "--play " + id,
		WorkDir:     filepath.Dir(launcher),
		Icon:        game.Executable,
		Description: game.Title,
	}
	if err := s.shortcuts.Create(path, link); err != nil {
		return err
	}

	previous := game.ShortcutPath
	game.ShortcutPath = path
	if err := s.persist(); err != nil {
		game.ShortcutPath = previous
		if previous == "" || !strings.EqualFold(previous, path) {
			if rollback := s.shortcuts.Remove(path); rollback != nil {
				return errors.Join(err, rollback)
			}
		}
		return err
	}
	if previous != "" && !strings.EqualFold(previous, path) {
		// Переименование игры сдвигает имя файла ярлыка; старый файл уже не
		// нужен, но его пропажа не повод отменять успешно созданный новый.
		if err := s.shortcuts.Remove(previous); err != nil {
			slog.Warn("remove stale shortcut", "id", id, "path", previous, "error", err)
		}
	}
	slog.Info("shortcut created", "id", id, "title", game.Title, "path", path)
	s.emitUpdated()
	return nil
}

func (s *Service) RemoveShortcut(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	game := s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	if game.ShortcutPath == "" {
		return nil
	}
	previous := game.ShortcutPath
	if err := s.shortcuts.Remove(previous); err != nil {
		return err
	}
	game.ShortcutPath = ""
	if err := s.persist(); err != nil {
		game.ShortcutPath = previous
		return err
	}
	slog.Info("shortcut removed", "id", id, "path", previous)
	s.emitUpdated()
	return nil
}

// dropShortcutLocked убирает ярлык вместе с установкой. Заблокированный файл
// ярлыка не повод отказать в удалении игры, поэтому путь остаётся в записи:
// по нему удаление можно повторить, а очистить поле, потеряв путь, — нельзя.
func (s *Service) dropShortcutLocked(game *Game) {
	if game.ShortcutPath == "" {
		return
	}
	if err := s.shortcuts.Remove(game.ShortcutPath); err != nil {
		slog.Warn("remove shortcut", "id", game.ID, "path", game.ShortcutPath, "error", err)
		return
	}
	game.ShortcutPath = ""
}

// safeCommandLineID отвергает идентификаторы, которые в аргументах ярлыка
// разъехались бы на несколько аргументов или изменили смысл командной строки.
func safeCommandLineID(id string) bool {
	if id == "" {
		return false
	}
	// Идентификатор, начинающийся с дефиса, разбирается как ещё один флаг
	// командной строки, а не как значение --play.
	if strings.HasPrefix(id, "-") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
