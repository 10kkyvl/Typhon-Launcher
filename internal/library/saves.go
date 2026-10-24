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
	"typhon/internal/steamlink"
	"typhon/internal/uierr"
)

var (
	errSavesEmptyPath = uierr.New("library.no_saves_path", "не указана папка сохранений")
	errSavesNotDir    = uierr.New("library.saves_not_a_directory", "это не папка")
)

// SavesResult описывает, чем закончился поиск сохранений: Path — папка, которую
// можно открывать сразу, Candidates — найденные варианты, между которыми
// выбирает пользователь. Unreadable считает каталоги, которые обход не смог
// прочитать: пустой результат при Unreadable > 0 не означает, что сохранений нет.
type SavesResult struct {
	Path       string   `json:"path"`
	Candidates []string `json:"candidates"`
	Unreadable int      `json:"unreadable"`
	// HintsError — почему подсказки путей не сработали (сеть, сервер). Пустая
	// строка значит, что подсказки либо отработали, либо их не запрашивали.
	HintsError string `json:"hintsError,omitempty"`
	// Hint заполнен, только когда Path или Candidates получены из подсказок
	// сервера, а не из SavesDir и не эвристиками: по нему окно показывает
	// атрибуцию источника.
	Hint *SaveHintSource `json:"hint,omitempty"`
}

// SaveHintSource — откуда взяты подсказки путей и на каких условиях.
type SaveHintSource struct {
	Source  string `json:"source"`
	License string `json:"license"`
	Via     string `json:"via"`
	PageURL string `json:"pageUrl"`
}

// SaveHintSet — шаблоны путей игры вместе с атрибуцией источника.
type SaveHintSet struct {
	Templates []string
	Source    SaveHintSource
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
	canonicalID := game.CanonicalGameID
	roots := s.saveRoots
	hints := s.saveHintSource()
	s.mu.Unlock()

	if stored != "" {
		info, err := os.Stat(stored)
		switch {
		case err == nil && info.IsDir():
			return SavesResult{Path: stored}, nil
		case err == nil:
			return SavesResult{}, fmt.Errorf("%w: %s", errSavesNotDir, stored)
		case !errors.Is(err, fs.ErrNotExist):
			return SavesResult{}, uierr.Wrap("library.saves_path_unavailable", fmt.Errorf("папка сохранений %s: %w", stored, err))
		}
	}

	var hinted hintOutcome
	if hints.lookup != nil && canonicalID != "" {
		var err error
		hinted, err = resolveHints(ctx, hints, canonicalID, installDir)
		if err != nil {
			return SavesResult{}, err
		}
		if len(hinted.paths) == 1 {
			return SavesResult{Path: hinted.paths[0], Unreadable: hinted.unreadable, HintsError: hinted.failure, Hint: &hinted.source}, nil
		}
		if len(hinted.paths) > 1 {
			return SavesResult{Candidates: hinted.paths, Unreadable: hinted.unreadable, HintsError: hinted.failure, Hint: &hinted.source}, nil
		}
	}

	result, err := detectSaves(ctx, title, installDir, roots)
	if err != nil {
		return SavesResult{}, err
	}
	result.Unreadable += hinted.unreadable
	result.HintsError = hinted.failure
	return result, nil
}

type hintSource struct {
	lookup    func(ctx context.Context, canonicalID string) (SaveHintSet, error)
	envs      func() ([]platform.SaveEnv, error)
	steamRoot func() (string, error)
}

func (s *Service) saveHintSource() hintSource {
	return hintSource{lookup: s.saveHints, envs: s.saveEnvs, steamRoot: s.steamRoot}
}

type hintOutcome struct {
	paths      []string
	source     SaveHintSource
	unreadable int
	failure    string
}

// resolveHints раскрывает шаблоны PCGamingWiki в существующие каталоги. Сбой
// получения шаблонов (сеть, сервер) не отменяет поиск: он уходит в failure, и
// пользователь видит причину рядом с результатом эвристик. Отмена ctx и
// ошибка чтения диска, наоборот, поднимаются наверх.
func resolveHints(ctx context.Context, src hintSource, canonicalID, installDir string) (hintOutcome, error) {
	var out hintOutcome
	set, lookupErr := src.lookup(ctx, canonicalID)
	templates := set.Templates
	out.source = set.Source
	if ctxErr := ctx.Err(); ctxErr != nil {
		return hintOutcome{}, ctxErr
	}
	// The failure is not dropped: it travels to the caller in the result.
	if out.failure = failureText(lookupErr); out.failure != "" {
		return out, nil
	}
	if len(templates) == 0 {
		return out, nil
	}
	envs, err := src.envs()
	if err != nil {
		return hintOutcome{}, fmt.Errorf("save hint folders: %w", err)
	}
	steam, err := steamRootFor(src.steamRoot)
	if err != nil {
		return hintOutcome{}, err
	}

	seen := map[string]bool{}
	for _, env := range envs {
		scoped := make(platform.SaveEnv, len(env)+2)
		for k, v := range env {
			scoped[k] = v
		}
		if installDir != "" {
			scoped[platform.TokenGame] = installDir
		}
		if _, ok := scoped[platform.TokenSteam]; !ok && steam != "" {
			scoped[platform.TokenSteam] = steam
		}
		for _, tmpl := range templates {
			found, unreadable, err := platform.ResolveSaveHint(ctx, tmpl, scoped)
			if errors.Is(err, platform.ErrSaveTokenUnavail) || errors.Is(err, platform.ErrBadSaveHint) {
				continue
			}
			if errors.Is(err, platform.ErrSaveHintTooBroad) {
				out.failure = err.Error()
				continue
			}
			if err != nil {
				return hintOutcome{}, fmt.Errorf("save hint %q: %w", tmpl, err)
			}
			out.unreadable += unreadable
			for _, p := range found {
				key, err := dedupKey(p)
				if err != nil {
					return hintOutcome{}, err
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				out.paths = append(out.paths, p)
			}
		}
	}
	return out, nil
}

// dedupKey сводит разные пути к одной папке, в том числе через симлинк
// (Documents бутыля CrossOver указывает на ~/Documents). Ошибка EvalSymlinks
// оставляет исходный путь: обход всё равно проверит его сам.
func dedupKey(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return platform.PathKey(p)
}

func failureText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// steamRootFor отличает «Steam не установлен» от поломки: первое означает
// только, что шаблоны с {steam} пропускаются.
func steamRootFor(root func() (string, error)) (string, error) {
	if root == nil {
		return "", nil
	}
	path, err := root()
	switch {
	case err == nil:
		return path, nil
	case errors.Is(err, steamlink.ErrNotInstalled), errors.Is(err, steamlink.ErrUnsupported):
		return "", nil
	default:
		return "", fmt.Errorf("steam root: %w", err)
	}
}

// SetSaveHints wires the lookup of save path templates for a catalog game; a
// game the lookup knows nothing about answers with no templates and no error.
//
//wails:ignore
func (s *Service) SetSaveHints(fn func(ctx context.Context, canonicalID string) (SaveHintSet, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveHints = fn
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
		return Game{}, uierr.Wrap("library.saves_path_unavailable", fmt.Errorf("папка сохранений %s: %w", normalized, err))
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
		verdict, err := vetHeuristicSaves(ctx, path)
		if err != nil {
			return err
		}
		result.Unreadable += verdict.unreadable
		if verdict.reason != "" {
			logRejectedSaves(path, verdict.reason)
			return nil
		}
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
