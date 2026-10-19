package platform

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
)

const (
	TokenGame            = "game"
	TokenSteam           = "steam"
	TokenUserProfile     = "userprofile"
	TokenDocuments       = "documents"
	TokenAppData         = "appdata"
	TokenLocalAppData    = "localappdata"
	TokenLocalAppDataLow = "localappdatalow"
	TokenSavedGames      = "savedgames"
	TokenPublic          = "public"
	TokenProgramData     = "programdata"

	maxHintMatches = 256
)

var SaveHintTokens = []string{
	TokenGame,
	TokenSteam,
	TokenUserProfile,
	TokenDocuments,
	TokenAppData,
	TokenLocalAppData,
	TokenLocalAppDataLow,
	TokenSavedGames,
	TokenPublic,
	TokenProgramData,
}

var (
	ErrBadSaveHint         = errors.New("некорректный шаблон пути сохранений")
	ErrSaveTokenUnavail    = errors.New("токен пути сохранений недоступен на этой машине")
	ErrSaveHintTooBroad    = errors.New("шаблон пути сохранений совпал со слишком большим числом каталогов")
	errSaveHintUnreadable  = errors.New("каталог не прочитан")
	errSaveHintNoTokenVal  = errors.New("пустое значение токена")
	errSaveHintNotAbsolute = errors.New("значение токена не абсолютный путь")
)

// SaveEnv держит значения токенов одной среды, где игра может хранить файлы:
// на Windows она одна, на macOS это каждый бутыль CrossOver. Токена нет в
// карте, когда в этой среде его значения не существует.
type SaveEnv map[string]string

// SplitSaveHint разбирает шаблон вида {token}/segment/*/segment. Это единственное
// место, которое знает синтаксис: проверка ответа сервера и раскрытие пути
// опираются на одни и те же правила.
func SplitSaveHint(hint string) (token string, segments []string, err error) {
	if !strings.HasPrefix(hint, "{") {
		return "", nil, fmt.Errorf("%w: нет токена в начале %q", ErrBadSaveHint, hint)
	}
	end := strings.IndexByte(hint, '}')
	if end < 0 {
		return "", nil, fmt.Errorf("%w: не закрыт токен в %q", ErrBadSaveHint, hint)
	}
	token = hint[1:end]
	known := false
	for _, t := range SaveHintTokens {
		if t == token {
			known = true
			break
		}
	}
	if !known {
		return "", nil, fmt.Errorf("%w: неизвестный токен %q", ErrBadSaveHint, token)
	}
	rest := hint[end+1:]
	if !strings.HasPrefix(rest, "/") {
		return "", nil, fmt.Errorf("%w: после токена нет разделителя в %q", ErrBadSaveHint, hint)
	}
	for _, seg := range strings.Split(rest, "/") {
		if seg == "" {
			continue
		}
		if seg == "." || seg == ".." {
			return "", nil, fmt.Errorf("%w: сегмент %q в %q", ErrBadSaveHint, seg, hint)
		}
		if strings.ContainsAny(seg, ":\\{}?\"<>|") {
			return "", nil, fmt.Errorf("%w: недопустимый символ в сегменте %q", ErrBadSaveHint, seg)
		}
		for _, r := range seg {
			if unicode.IsControl(r) {
				return "", nil, fmt.Errorf("%w: управляющий символ в %q", ErrBadSaveHint, hint)
			}
		}
		segments = append(segments, seg)
	}
	if !slices.ContainsFunc(segments, func(seg string) bool { return strings.Trim(seg, "*") != "" }) {
		return "", nil, fmt.Errorf("%w: %q указывает на весь корень токена", ErrBadSaveHint, hint)
	}
	return token, segments, nil
}

// ResolveSaveHint раскрывает шаблон в существующие каталоги. Токен без значения
// в env даёт ErrSaveTokenUnavail: шаблон пропускают, это не сбой. unreadable
// считает каталоги, которые не отдали содержимое, и ссылки (симлинк, junction),
// по которым обход не идёт: пустой ответ при unreadable > 0 не означает, что
// каталогов нет. Ссылка увела бы выгрузку сохранений за пределы папки игры.
func ResolveSaveHint(ctx context.Context, hint string, env SaveEnv) (paths []string, unreadable int, err error) {
	token, segments, err := SplitSaveHint(hint)
	if err != nil {
		return nil, 0, err
	}
	base, ok := env[token]
	if !ok {
		return nil, 0, fmt.Errorf("%w: {%s}", ErrSaveTokenUnavail, token)
	}
	if strings.TrimSpace(base) == "" {
		return nil, 0, fmt.Errorf("%w: {%s}", errSaveHintNoTokenVal, token)
	}
	if !filepath.IsAbs(base) {
		return nil, 0, fmt.Errorf("%w: {%s} = %q", errSaveHintNotAbsolute, token, base)
	}

	root := filepath.Clean(base)
	if token == TokenGame {
		info, lstatErr := os.Lstat(root)
		switch {
		case lstatErr == nil:
			if IsLinkMode(info.Mode()) {
				return nil, 1, nil
			}
		case errors.Is(lstatErr, fs.ErrNotExist):
			return nil, 0, nil
		case errors.Is(lstatErr, fs.ErrPermission):
			return nil, 1, nil
		default:
			return nil, 0, fmt.Errorf("lstat %s: %w", root, lstatErr)
		}
	}
	current := []string{root}
	for _, seg := range segments {
		next := make([]string, 0, len(current))
		for _, dir := range current {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			if !strings.Contains(seg, "*") {
				literal := filepath.Join(dir, seg)
				info, lstatErr := os.Lstat(literal)
				switch {
				case lstatErr == nil:
					if IsLinkMode(info.Mode()) {
						unreadable++
					} else if info.IsDir() {
						next = append(next, literal)
					}
				case errors.Is(lstatErr, fs.ErrNotExist):
				case errors.Is(lstatErr, fs.ErrPermission):
					unreadable++
				default:
					return nil, 0, fmt.Errorf("lstat %s: %w", literal, lstatErr)
				}
				continue
			}
			names, skipped, err := matchChildren(dir, seg)
			if err != nil {
				return nil, 0, err
			}
			unreadable += skipped
			for _, name := range names {
				next = append(next, filepath.Join(dir, name))
			}
		}
		if len(next) > maxHintMatches {
			return nil, 0, fmt.Errorf("%w: %q", ErrSaveHintTooBroad, hint)
		}
		current = next
	}

	for _, candidate := range current {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		info, statErr := os.Stat(candidate)
		switch {
		case statErr == nil:
			if info.IsDir() {
				paths = append(paths, candidate)
			}
		case errors.Is(statErr, fs.ErrNotExist):
		case errors.Is(statErr, fs.ErrPermission):
			unreadable++
		default:
			return nil, 0, fmt.Errorf("stat %s: %w", candidate, statErr)
		}
	}
	return paths, unreadable, nil
}

// IsLinkMode узнаёт симлинк и junction: Go отдаёт junction как ModeIrregular.
func IsLinkMode(m fs.FileMode) bool {
	return m&(fs.ModeSymlink|fs.ModeIrregular) != 0
}

func matchChildren(dir, pattern string) (names []string, unreadable int, err error) {
	entries, readErr := os.ReadDir(dir)
	switch {
	case readErr == nil:
	case errors.Is(readErr, fs.ErrNotExist):
		return nil, 0, nil
	case errors.Is(readErr, fs.ErrPermission):
		return nil, 1, nil
	default:
		return nil, 0, fmt.Errorf("%w: %s: %w", errSaveHintUnreadable, dir, readErr)
	}
	lowered := strings.ToLower(pattern)
	for _, entry := range entries {
		if !wildMatch(lowered, strings.ToLower(entry.Name())) {
			continue
		}
		// ReadDir не выдаёт junction как ссылку, поэтому режим берётся Lstat.
		info, lstatErr := os.Lstat(filepath.Join(dir, entry.Name()))
		switch {
		case errors.Is(lstatErr, fs.ErrNotExist):
			continue
		case errors.Is(lstatErr, fs.ErrPermission):
			unreadable++
			continue
		case lstatErr != nil:
			return nil, 0, fmt.Errorf("lstat %s: %w", entry.Name(), lstatErr)
		}
		if IsLinkMode(info.Mode()) {
			unreadable++
			continue
		}
		if !info.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, unreadable, nil
}

// wildMatch знает единственный спецсимвол — *, любая последовательность
// символов внутри одного сегмента; скобки и вопросительные знаки в именах
// папок остаются обычными символами.
func wildMatch(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(name, mid)
		if i < 0 {
			return false
		}
		name = name[i+len(mid):]
	}
	return strings.HasSuffix(name, last)
}
