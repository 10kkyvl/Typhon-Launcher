package library

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"typhon/internal/platform"
)

var (
	errOutsideInstall = errors.New("файл находится вне папки установки")
	errNoExecutable   = errors.New("исполняемый файл не найден")
)

type Outcome string

const (
	OutcomeCreated   Outcome = "created"
	OutcomeUpdated   Outcome = "updated"
	OutcomeUnchanged Outcome = "unchanged"
	OutcomeIgnored   Outcome = "ignored"
)

type Discovered struct {
	GameID          string
	Title           string
	Executable      string
	InstallDir      string
	Version         string
	VersionSource   string
	CanonicalGameID string
	SizeBytes       int64
	SizeUnknown     bool
}

// markInstalled оставляет метку рядом с установкой. Каталог может быть закрыт
// на запись (внешний установщик отработал от администратора), и это не повод
// считать установку несостоявшейся: без метки повторное сканирование опознает
// игру обычным поиском по каталогу.
func markInstalled(game Game) {
	if game.InstallDir == "" || game.ID == "" {
		return
	}
	if err := WriteMarker(game.InstallDir, markerFor(game)); err != nil {
		slog.Warn("write install marker", "id", game.ID, "dir", game.InstallDir, "error", err)
	}
}

func (s *Service) ApplyDiscovered(d Discovered) (Game, Outcome, error) {
	installDir := strings.TrimSpace(d.InstallDir)
	if installDir == "" {
		return Game{}, "", errEmptyInstallDir
	}
	key, err := platform.PathKey(installDir)
	if err != nil {
		return Game{}, "", fmt.Errorf("normalize %s: %w", installDir, err)
	}
	executable := strings.TrimSpace(d.Executable)
	if executable != "" {
		info, err := os.Stat(executable)
		if err != nil {
			return Game{}, "", fmt.Errorf("%w: %w", errNoExecutable, err)
		}
		if info.IsDir() {
			return Game{}, "", errNoExecutable
		}
		if !platform.Inside(installDir, executable) {
			return Game{}, "", errOutsideInstall
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ignored, err := s.excludedLocked(installDir)
	if err != nil {
		return Game{}, "", err
	}
	if ignored {
		return Game{}, OutcomeIgnored, nil
	}

	pos, err := s.matchDiscoveredLocked(d.GameID, key, executable)
	if err != nil {
		return Game{}, "", err
	}
	if pos < 0 {
		return s.createDiscoveredLocked(d, installDir, executable)
	}
	return s.mergeDiscoveredLocked(pos, d, installDir, executable)
}

func (s *Service) matchDiscoveredLocked(gameID, key, executable string) (int, error) {
	byPath, byID, byExecutable := -1, -1, -1
	for i := range s.games {
		if s.games[i].InstallDir != "" && byPath < 0 {
			existing, err := platform.PathKey(s.games[i].InstallDir)
			if err != nil {
				return -1, fmt.Errorf("normalize %s: %w", s.games[i].InstallDir, err)
			}
			if existing == key {
				byPath = i
			}
		}
		if gameID != "" && s.games[i].ID == gameID {
			byID = i
		}
		if executable != "" && byExecutable < 0 && strings.EqualFold(s.games[i].Executable, executable) {
			byExecutable = i
		}
	}
	switch {
	case byID >= 0 && (byPath < 0 || byPath == byID):
		return byID, nil
	case byPath >= 0:
		return byPath, nil
	default:
		return byExecutable, nil
	}
}

func (s *Service) createDiscoveredLocked(d Discovered, installDir, executable string) (Game, Outcome, error) {
	title := strings.TrimSpace(d.Title)
	if title == "" && executable != "" {
		title = TitleFromExecutable(executable)
	}
	if title == "" {
		return Game{}, "", errors.New("не удалось определить название игры")
	}
	game := Game{
		ID:              newID(),
		Title:           title,
		Executable:      executable,
		InstallDir:      installDir,
		Version:         d.Version,
		VersionSource:   d.VersionSource,
		SizeBytes:       d.SizeBytes,
		SizeUnknown:     d.SizeUnknown,
		InstalledAt:     time.Now(),
		CanonicalGameID: d.CanonicalGameID,
		Source:          SourceDiscovered,
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, "", fmt.Errorf("save library: %w", err)
	}
	slog.Info("game discovered", "id", game.ID, "title", game.Title, "dir", game.InstallDir)
	s.emitUpdated()
	return game, OutcomeCreated, nil
}

func (s *Service) mergeDiscoveredLocked(pos int, d Discovered, installDir, executable string) (Game, Outcome, error) {
	previous := s.games[pos]
	next := previous
	changed := false

	if !platform.SamePath(next.InstallDir, installDir) {
		next.InstallDir = installDir
		changed = true
	}
	if executable != "" && !strings.EqualFold(next.Executable, executable) {
		next.Executable = executable
		changed = true
	}
	if next.Title == "" && d.Title != "" {
		next.Title = d.Title
		changed = true
	}
	if next.CanonicalGameID == "" && d.CanonicalGameID != "" {
		next.CanonicalGameID = d.CanonicalGameID
		changed = true
	}
	if next.Version == "" && d.Version != "" {
		next.Version = d.Version
		next.VersionSource = d.VersionSource
		changed = true
	}
	if next.Source == "" {
		next.Source = SourceManaged
	}
	if next.Uninstalled {
		next.Uninstalled = false
		changed = true
	}

	touched := changed || next.SizeBytes != d.SizeBytes || next.SizeUnknown != d.SizeUnknown
	next.SizeBytes = d.SizeBytes
	next.SizeUnknown = d.SizeUnknown
	if !touched {
		return previous, OutcomeUnchanged, nil
	}

	s.games[pos] = next
	if err := s.persist(); err != nil {
		s.games[pos] = previous
		return Game{}, "", fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	if !changed {
		return next, OutcomeUnchanged, nil
	}
	slog.Info("game rediscovered", "id", next.ID, "title", next.Title, "dir", next.InstallDir)
	return next, OutcomeUpdated, nil
}

func (s *Service) SetExecutable(id, executable string) (Game, error) {
	executable = strings.TrimSpace(executable)
	info, err := os.Stat(executable)
	if err != nil {
		return Game{}, fmt.Errorf("%w: %w", errNoExecutable, err)
	}
	if info.IsDir() {
		return Game{}, errNoExecutable
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID != id {
			continue
		}
		if s.games[i].InstallDir != "" && !platform.Inside(s.games[i].InstallDir, executable) {
			return Game{}, errOutsideInstall
		}
		if strings.EqualFold(s.games[i].Executable, executable) {
			return s.games[i], nil
		}
		previous := s.games[i]
		s.games[i].Executable = executable
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		slog.Info("game executable set", "id", id, "executable", executable)
		s.emitUpdated()
		return s.games[i], nil
	}
	return Game{}, errNotFound
}
