package install

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"typhon/internal/emulators"
	"typhon/internal/library"
)

// sharesFootprint says whether path overlaps what the game occupies on disk,
// which for an emulated game is its files and not the folder they lie in.
func sharesFootprint(game library.Game, path string) bool {
	return slices.ContainsFunc(game.Footprint(), func(root string) bool { return sharesPath(root, path) })
}

// checkHoldsNoImage refuses to delete or consume a folder that an emulated game
// of the library keeps its files in: they are not part of what is being removed.
func (s *Service) checkHoldsNoImage(dir string) error {
	if s.library == nil {
		return errNoLibrary
	}
	for _, game := range s.library.GetGames() {
		if game.Emulation != nil && sharesFootprint(game, dir) {
			return fmt.Errorf("%w: %s", emulators.ErrImageInside, game.Title)
		}
	}
	return nil
}

func (s *Service) gameRoots() []string {
	var roots []string
	if s.library != nil {
		for _, game := range s.library.GetGames() {
			roots = append(roots, game.Footprint()...)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.Destination != "" && (active(item.Status) || item.Status == StatusCompleted) {
			roots = append(roots, item.Destination)
		}
	}
	return roots
}

func (s *Service) insideGame(dest string) bool {
	for _, root := range s.gameRoots() {
		if inside(root, dest) && !samePath(root, dest) {
			return true
		}
	}
	return false
}

// suggestDestination выбирает соседа, а не вложенную папку, когда dest сам
// стоит в чужой игре или носит имя этой игры: подпапка dest\Name оказалась бы
// внутри другой установки, и удаление одной игры унесло бы вторую.
func (s *Service) suggestDestination(dest, name string) (string, error) {
	roots := s.gameRoots()
	parent := dest
	switch {
	case insideAny(roots, dest):
		for insideAny(roots, parent) {
			up := filepath.Dir(parent)
			if up == parent {
				break
			}
			parent = up
		}
	case strings.EqualFold(filepath.Base(dest), sanitizeName(name)):
		parent = filepath.Dir(dest)
	default:
		return s.proposeDestination(dest, name)
	}
	return s.proposeDestination(parent, name)
}

func (s *Service) proposeDestination(root, name string) (string, error) {
	if root == "" {
		return "", nil
	}
	base := filepath.Join(root, sanitizeName(name))
	var unreadable error
	var unreadablePath string
	readable := false
	for i := 1; i < maxSuffixAttempt; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d)", base, i)
		}
		state, err := s.checkDest(candidate)
		if err != nil {
			if unreadable == nil {
				slog.Warn("skip a subfolder name that cannot be checked", "path", candidate, "error", err)
			}
			unreadable, unreadablePath = err, candidate
			continue
		}
		if state == destMissing || state == destEmpty {
			return candidate, nil
		}
		readable = true
	}
	if !readable {
		return "", fmt.Errorf("%w: %s: %w", errDestUnreadable, unreadablePath, unreadable)
	}
	return "", fmt.Errorf("%w: %s", errNoFreeFolder, base)
}
