package library

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/uierr"
)

const (
	maxLaunchArgs     = 32
	maxLaunchArgChars = 512
)

var (
	errLaunchArgsInvalid = uierr.New("library.launch_args_invalid", "параметры запуска содержат недопустимые символы или незакрытую кавычку")
	errLaunchArgsTooLong = uierr.New("library.launch_args_too_long", "параметры запуска слишком длинные")
)

func (s *Service) SetLaunchArgs(id string, args []string) (Game, error) {
	normalized, err := normalizeLaunchArgs(args)
	if err != nil {
		return Game{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if equalStrings(game.LaunchArgs, normalized) {
		return *game, nil
	}
	previous := *game
	game.LaunchArgs = normalized
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) SetLaunchArgsLine(id, line string) (Game, error) {
	args, err := ParseLaunchArgs(line)
	if err != nil {
		return Game{}, err
	}
	return s.SetLaunchArgs(id, args)
}

func (s *Service) GetLaunchArgsLine(id string) (string, error) {
	game, err := s.Find(id)
	if err != nil {
		return "", err
	}
	return LaunchArgsLine(game.LaunchArgs), nil
}

func (s *Service) SetRunAsAdmin(id string, on bool) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.RunAsAdmin == on {
		return *game, nil
	}
	previous := *game
	game.RunAsAdmin = on
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) SetHidePresence(id string, on bool) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.HidePresence == on {
		return *game, nil
	}
	previous := *game
	game.HidePresence = on
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	if sess, ok := s.running[id]; ok {
		for _, w := range s.watchers {
			if cw, ok := w.(SessionChangeWatcher); ok {
				cw.SessionChanged(*game, sess.startedAt)
			}
		}
	}
	return *game, nil
}

func (s *Service) ClearSavesDir(id string) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.SavesDir == "" {
		return *game, nil
	}
	previous := *game
	game.SavesDir = ""
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func normalizeLaunchArgs(args []string) ([]string, error) {
	var out []string
	total := 0
	for _, arg := range args {
		for _, r := range arg {
			if unicode.IsControl(r) {
				return nil, errLaunchArgsInvalid
			}
		}
		total += utf8.RuneCountInString(arg)
		out = append(out, arg)
		if len(out) > maxLaunchArgs || total > maxLaunchArgChars {
			return nil, errLaunchArgsTooLong
		}
	}
	return out, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ParseLaunchArgs разбирает строку по правилам CommandLineToArgvW: пробелы
// разделяют аргументы, кавычки склеивают пробелы, \" — литеральная кавычка.
func ParseLaunchArgs(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	started := false
	inQuote := false
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			n := 0
			for i < len(runes) && runes[i] == '\\' {
				n++
				i++
			}
			if i < len(runes) && runes[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				started = true
				if n%2 == 1 {
					cur.WriteRune('"')
				} else {
					inQuote = !inQuote
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
				started = true
				i--
			}
		case r == '"':
			inQuote = !inQuote
			started = true
		case !inQuote && (r == ' ' || r == '\t'):
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		case unicode.IsControl(r) && r != '\t':
			return nil, errLaunchArgsInvalid
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inQuote {
		return nil, errLaunchArgsInvalid
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}

// LaunchArgsLine — обратное к ParseLaunchArgs: аргументы с пробелом, табом,
// кавычкой или пустые берутся в кавычки.
func LaunchArgsLine(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = quoteLaunchArg(arg)
	}
	return strings.Join(parts, " ")
}

func quoteLaunchArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
			b.WriteRune(r)
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			backslashes = 0
			b.WriteRune(r)
		}
	}
	b.WriteString(strings.Repeat(`\`, backslashes))
	b.WriteByte('"')
	return b.String()
}
