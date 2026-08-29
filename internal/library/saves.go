package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"typhon/internal/platform"
)

var (
	errSavesEmptyPath = errors.New("не указана папка сохранений")
	errSavesNotDir    = errors.New("это не папка")
)

// SavesResult описывает, чем закончился поиск сохранений: Path — папка, которую
// можно открывать сразу, Candidates — найденные варианты, между которыми
// выбирает пользователь. Unreadable считает каталоги, которые обход не смог
// прочитать: пустой результат при Unreadable > 0 не означает, что сохранений нет.
type SavesResult struct {
	Path       string   `json:"path"`
	Candidates []string `json:"candidates"`
	Unreadable int      `json:"unreadable"`
}

var installSaveDirs = []string{
	"save",
	"saves",
	"saved",
	"savedgames",
	"savedata",
	"savegame",
	"savegames",
	"profiles",
	"userdata",
}

const minTitlePrefix = 5

func (s *Service) LocateSaves(ctx context.Context, id string) (SavesResult, error) {
	s.mu.Lock()
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return SavesResult{}, errNotFound
	}
	stored, title, installDir := game.SavesDir, game.Title, game.InstallDir
	roots := s.saveRoots
	s.mu.Unlock()

	if stored != "" {
		info, err := os.Stat(stored)
		switch {
		case err == nil && info.IsDir():
			return SavesResult{Path: stored}, nil
		case err == nil:
			return SavesResult{}, fmt.Errorf("%w: %s", errSavesNotDir, stored)
		case !errors.Is(err, fs.ErrNotExist):
			return SavesResult{}, fmt.Errorf("папка сохранений %s: %w", stored, err)
		}
	}

	return detectSaves(ctx, title, installDir, roots)
}

func (s *Service) SetSavesDir(id, dir string) (Game, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return Game{}, errSavesEmptyPath
	}
	normalized, err := platform.Normalize(dir)
	if err != nil {
		return Game{}, err
	}
	info, err := os.Stat(normalized)
	if err != nil {
		return Game{}, fmt.Errorf("папка сохранений %s: %w", normalized, err)
	}
	if !info.IsDir() {
		return Game{}, fmt.Errorf("%w: %s", errSavesNotDir, normalized)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID != id {
			continue
		}
		if s.games[i].SavesDir == normalized {
			return s.games[i], nil
		}
		previous := s.games[i]
		s.games[i].SavesDir = normalized
		if err := s.persist(); err != nil {
			s.games[i] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
		s.emitUpdated()
		return s.games[i], nil
	}
	return Game{}, errNotFound
}

func detectSaves(ctx context.Context, title, installDir string, roots func() ([]platform.SaveRoot, error)) (SavesResult, error) {
	result := SavesResult{}
	seen := map[string]bool{}
	add := func(path string) error {
		key, err := platform.PathKey(path)
		if err != nil {
			return err
		}
		if seen[key] {
			return nil
		}
		seen[key] = true
		result.Candidates = append(result.Candidates, path)
		return nil
	}

	if installDir != "" {
		entries, unreadable, err := readDirEntries(installDir)
		if err != nil {
			return SavesResult{}, err
		}
		result.Unreadable += unreadable
		for _, entry := range entries {
			if !entry.IsDir() || !slices.Contains(installSaveDirs, titleKey(entry.Name())) {
				continue
			}
			if err := add(filepath.Join(installDir, entry.Name())); err != nil {
				return SavesResult{}, err
			}
		}
	}

	key := titleKey(title)
	if key == "" {
		return result, nil
	}
	if roots == nil {
		roots = platform.SaveRoots
	}
	list, err := roots()
	if err != nil {
		return SavesResult{}, err
	}
	for _, root := range list {
		unreadable, err := scanSaveRoot(ctx, root.Path, root.Depth, key, add)
		if err != nil {
			return SavesResult{}, err
		}
		result.Unreadable += unreadable
	}

	if len(result.Candidates) == 1 {
		result.Path = result.Candidates[0]
		result.Candidates = nil
	}
	return result, nil
}

func scanSaveRoot(ctx context.Context, root string, depth int, key string, add func(string) error) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	entries, unreadable, err := readDirEntries(root)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if matchesTitle(entry.Name(), key) {
			if err := add(path); err != nil {
				return 0, err
			}
			continue
		}
		if depth <= 1 {
			continue
		}
		deeper, err := scanSaveRoot(ctx, path, depth-1, key, add)
		if err != nil {
			return 0, err
		}
		unreadable += deeper
	}
	return unreadable, nil
}

// readDirEntries отделяет «каталога нет» и «каталог не отдали» от настоящей
// ошибки: под AppData регулярно попадаются папки без прав доступа, и одна такая
// не должна отменять весь поиск — она попадает в счётчик Unreadable, который
// доезжает до пользователя.
func readDirEntries(dir string) ([]fs.DirEntry, int, error) {
	entries, err := os.ReadDir(dir)
	unreadable, fatal := classifyDirErr(err)
	if fatal {
		return nil, 0, fmt.Errorf("read %s: %w", dir, err)
	}
	return entries, unreadable, nil
}

func classifyDirErr(err error) (unreadable int, fatal bool) {
	switch {
	case err == nil, errors.Is(err, fs.ErrNotExist):
		return 0, false
	case errors.Is(err, fs.ErrPermission):
		return 1, false
	default:
		return 0, true
	}
}

func titleKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func matchesTitle(name, key string) bool {
	other := titleKey(name)
	if other == "" {
		return false
	}
	if other == key {
		return true
	}
	if len(other) >= minTitlePrefix && strings.HasPrefix(key, other) {
		return true
	}
	return len(key) >= minTitlePrefix && strings.HasPrefix(other, key)
}
