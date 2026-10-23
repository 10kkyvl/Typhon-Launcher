package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/shortcut"
	"typhon/internal/uierr"
)

var (
	errShortcutUnsupported  = uierr.New("library.shortcut_unsupported", "ярлыки поддерживаются только в Windows")
	errShortcutUninstalled  = uierr.New("library.not_installed", "игра не установлена")
	errShortcutNoExecutable = uierr.New("library.no_shortcut_executable", "у игры не задан исполняемый файл")
	errShortcutBadID        = uierr.New("library.shortcut_bad_id", "идентификатор игры непригоден для командной строки")
)

type shortcutBackend interface {
	Supported() bool
	DesktopDir() (string, error)
	FileName(title string) (string, error)
	Create(path string, link shortcut.Link) error
	Remove(path string) error
	SetAppID(path, launcher, appID string) error
}

type systemShortcuts struct{}

func (systemShortcuts) Supported() bool                        { return shortcut.Supported() }
func (systemShortcuts) DesktopDir() (string, error)            { return shortcut.DesktopDir() }
func (systemShortcuts) FileName(title string) (string, error)  { return shortcut.FileName(title) }
func (systemShortcuts) Create(p string, l shortcut.Link) error { return shortcut.Create(p, l) }
func (systemShortcuts) Remove(p string) error                  { return shortcut.Remove(p) }
func (systemShortcuts) SetAppID(p, launcher, appID string) error {
	return shortcut.SetLinkAppID(p, launcher, appID, shortcut.IsPlayArgs)
}

func (s *Service) CreateShortcut(id string) error {
	return s.CreateProfileShortcut(id, "")
}

func (s *Service) CreateProfileShortcut(id, profileID string) error {
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
	if profileID != "" && !safeCommandLineID(profileID) {
		return fmt.Errorf("%w: %q", errProfileBadID, profileID)
	}
	chosen, err := chooseProfile(game, profileID, true)
	if err != nil {
		return err
	}
	if _, err := os.Stat(chosen.executable); err != nil {
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
	title := game.Title
	if profileID != "" {
		title = profileTitle(*game, profileID)
	}
	name, err := s.shortcuts.FileName(title)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name)
	link, err := gameShortcutLink(*game, profileID, chosen.executable, launcher)
	if err != nil {
		return err
	}
	if err := s.shortcuts.Create(path, link); err != nil {
		return err
	}

	snapshot := *game
	previous := game.ShortcutPath
	if profileID == "" {
		game.ShortcutPath = path
	} else {
		game.Profiles = cloneProfiles(snapshot.Profiles)
		p := findProfile(game, profileID)
		previous = p.ShortcutPath
		p.ShortcutPath = path
	}
	if err := s.persist(); err != nil {
		*game = snapshot
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

// gameShortcutLink собирает ярлык запуска игры. Иконку Windows берёт прямо из
// исполняемого файла игры, поэтому отдельный .ico не нужен.
func gameShortcutLink(game Game, profileID, icon, launcher string) (shortcut.Link, error) {
	appID, args, description, err := shortcutIdentity(game, profileID)
	if err != nil {
		return shortcut.Link{}, err
	}
	return shortcut.Link{
		Target:         launcher,
		Args:           args,
		WorkDir:        filepath.Dir(launcher),
		Icon:           icon,
		Description:    description,
		AppUserModelID: appID,
	}, nil
}

// shortcutIdentity gives each launch profile its own AppUserModelID, so two
// profile shortcuts of one game do not share a taskbar group.
func shortcutIdentity(game Game, profileID string) (appID, args, description string, err error) {
	gameID := game.ID
	args, err = shortcut.PlayArgs(game.ID, profileID)
	if err != nil {
		return "", "", "", err
	}
	description = game.Title
	if profileID != "" {
		gameID += "." + profileID
		description = profileTitle(game, profileID)
	}
	appID, err = shortcut.GameAppID(gameID)
	if err != nil {
		return "", "", "", fmt.Errorf("идентификатор ярлыка игры %q: %w", game.ID, err)
	}
	return appID, args, description, nil
}

func (s *Service) startShortcutRefresh(ctx context.Context) {
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if closed {
		return
	}
	go func() {
		defer s.wg.Done()
		if err := s.refreshShortcuts(ctx); err != nil {
			slog.Warn("refresh game shortcuts", "error", err)
		}
		if s.launcherLinks == nil {
			return
		}
		if err := s.launcherLinks(ctx); err != nil {
			slog.Warn("refresh launcher shortcuts", "error", err)
		}
	}()
}

// refreshLauncherLinks проставляет AppUserModelID на собственных ярлыках
// лаунчера (меню Пуск, рабочий стол, закрепление на панели задач), чтобы окно
// группировалось с ними, а не с ярлыком игры.
func refreshLauncherLinks(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("путь к лаунчеру: %w", err)
	}
	spots, err := shortcut.LauncherLinkSpots()
	if err != nil {
		return err
	}
	return shortcut.RefreshLauncherLinks(ctx, spots, exe, shortcut.LauncherAppID)
}

// refreshShortcuts проставляет AppUserModelID на ярлыках, созданных до его
// появления, иначе оболочка продолжает брать значок игры для окна лаунчера.
// Ярлык правится на месте: значок, горячая клавиша и режим запуска остаются
// пользовательскими. Проверка записи и правка файла идут под одним захватом,
// чтобы удаление ярлыка не обогнало обновление и не оставило сироту.
func (s *Service) refreshShortcuts(ctx context.Context) error {
	s.mu.Lock()
	if !s.shortcuts.Supported() {
		s.mu.Unlock()
		return nil
	}
	var links []shortcutRef
	for _, g := range s.games {
		if g.Uninstalled || g.Executable == "" {
			continue
		}
		if g.ShortcutPath != "" {
			links = append(links, shortcutRef{gameID: g.ID, path: g.ShortcutPath})
		}
		for _, p := range g.Profiles {
			if p.ShortcutPath != "" {
				links = append(links, shortcutRef{gameID: g.ID, profileID: p.ID, path: p.ShortcutPath})
			}
		}
	}
	launcherPath := s.launcherPath
	s.mu.Unlock()
	if len(links) == 0 {
		return nil
	}
	launcher, err := launcherPath()
	if err != nil {
		return fmt.Errorf("путь к лаунчеру: %w", err)
	}

	var errs []error
	for _, ref := range links {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		appID, _, _, err := shortcutIdentity(Game{ID: ref.gameID}, ref.profileID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := s.setShortcutAppID(ref, launcher, appID); err != nil {
			errs = append(errs, fmt.Errorf("ярлык игры %q: %w", ref.gameID, err))
		}
	}
	return errors.Join(errs...)
}

type shortcutRef struct {
	gameID    string
	profileID string
	path      string
}

func (s *Service) setShortcutAppID(ref shortcutRef, launcher, appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.findLocked(ref.gameID)
	if current == nil || current.Uninstalled {
		return nil
	}
	path := current.ShortcutPath
	if ref.profileID != "" {
		p := findProfile(current, ref.profileID)
		if p == nil {
			return nil
		}
		path = p.ShortcutPath
	}
	if !strings.EqualFold(path, ref.path) {
		return nil
	}
	return s.shortcuts.SetAppID(path, launcher, appID)
}

// dropShortcutLocked убирает ярлык вместе с установкой. Заблокированный файл
// ярлыка не повод отказать в удалении игры, поэтому путь остаётся в записи:
// по нему удаление можно повторить, а очистить поле, потеряв путь, — нельзя.
func (s *Service) dropShortcutLocked(game *Game) {
	if game.Profiles != nil {
		game.Profiles = cloneProfiles(game.Profiles)
		for i := range game.Profiles {
			p := &game.Profiles[i]
			if p.ShortcutPath == "" {
				continue
			}
			if err := s.shortcuts.Remove(p.ShortcutPath); err != nil {
				slog.Warn("remove profile shortcut", "id", game.ID, "profile", p.ID, "path", p.ShortcutPath, "error", err)
				continue
			}
			p.ShortcutPath = ""
		}
	}
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
func safeCommandLineID(id string) bool { return shortcut.SafeCommandLineID(id) }
