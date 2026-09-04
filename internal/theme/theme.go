package theme

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"typhon/internal/uierr"
)

const (
	BaseDark  = "dark"
	BaseLight = "light"

	MaxThemeBytes = 256 << 10
	MaxCSSBytes   = 32 << 10

	maxNameRunes  = 64
	maxTokenValue = 120
	maxCSSDepth   = 4
)

type Theme struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Tokens    map[string]string `json:"tokens"`
	CSS       string            `json:"css,omitempty"`
	BuiltIn   bool              `json:"builtIn"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type file struct {
	Version int   `json:"version"`
	Theme   Theme `json:"theme"`
}

var (
	ErrThemeSize       = uierr.New("theme.file_too_large", "файл темы превышает допустимый размер")
	ErrThemeVersion    = uierr.New("theme.unsupported_version", "неподдерживаемая версия файла темы")
	ErrThemeID         = uierr.New("theme.invalid_id", "недопустимый идентификатор темы")
	ErrThemeName       = uierr.New("theme.invalid_name", "недопустимое имя темы")
	ErrThemeBase       = uierr.New("theme.invalid_base", "недопустимая базовая палитра темы")
	ErrThemeTokenName  = uierr.New("theme.unknown_token", "неизвестное имя токена темы")
	ErrThemeTokenValue = uierr.New("theme.invalid_token_value", "недопустимое значение токена темы")
	ErrThemeCSSSize    = uierr.New("theme.css_too_large", "пользовательский CSS превышает допустимый размер")
	ErrThemeCSSContent = uierr.New("theme.css_forbidden", "пользовательский CSS содержит запрещённую конструкцию")
	ErrThemeCSSBraces  = uierr.New("theme.css_unbalanced_braces", "непарные или слишком глубоко вложенные фигурные скобки в CSS")
	ErrThemeBuiltIn    = uierr.New("theme.built_in", "встроенную тему нельзя изменить или удалить")
	ErrThemeNotFound   = uierr.New("theme.not_found", "тема не найдена")
	ErrThemePath       = uierr.New("theme.invalid_path", "недопустимый путь к файлу темы")
)

var (
	idRe         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	tokenValueRe = regexp.MustCompile(`^[#0-9A-Za-z .,%()/+*-]+$`)
	cssComment   = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// url( and /* consist entirely of characters tokenValueRe already allows, so
// they need an explicit substring check on top of the charset regex.
var forbiddenTokenSubstrings = []string{"url(", "expression(", "/*"}

var forbiddenCSSSubstrings = []string{
	"@import", "url(", "expression(", "javascript:", "behavior:", "-moz-binding", "</style",
	// Масштаб интерфейса принадлежит настройке со своим переключателем, а не
	// теме: правило из пользовательского CSS дало бы значению второго владельца.
	"--ui-scale",
}

// Validate normalizes and checks a theme submitted by a caller (Save or
// Import). It never mutates the input in place; it returns the normalized
// copy on success.
func Validate(t Theme) (Theme, error) {
	id := strings.TrimSpace(t.ID)
	if !idRe.MatchString(id) {
		return Theme{}, fmt.Errorf("%q: %w", t.ID, ErrThemeID)
	}
	t.ID = id

	name, err := sanitizeName(t.Name)
	if err != nil {
		return Theme{}, err
	}
	t.Name = name

	switch t.Base {
	case BaseDark, BaseLight:
	default:
		return Theme{}, fmt.Errorf("%q: %w", t.Base, ErrThemeBase)
	}

	for name, value := range t.Tokens {
		if err := validateTokenName(name); err != nil {
			return Theme{}, err
		}
		if err := validateTokenValue(value); err != nil {
			return Theme{}, fmt.Errorf("%s: %w", name, err)
		}
	}

	if err := validateCSS(t.CSS); err != nil {
		return Theme{}, err
	}

	return t, nil
}

func sanitizeName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	runes := len([]rune(name))
	if runes == 0 || runes > maxNameRunes {
		return "", fmt.Errorf("%q: %w", raw, ErrThemeName)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%q: %w", raw, ErrThemeName)
		}
	}
	return name, nil
}

func validateTokenName(name string) error {
	if _, ok := allowedTokenSet[name]; !ok {
		return fmt.Errorf("%q: %w", name, ErrThemeTokenName)
	}
	return nil
}

func validateTokenValue(value string) error {
	if len(value) == 0 || len(value) > maxTokenValue {
		return ErrThemeTokenValue
	}
	if !tokenValueRe.MatchString(value) {
		return ErrThemeTokenValue
	}
	lower := strings.ToLower(value)
	for _, bad := range forbiddenTokenSubstrings {
		if strings.Contains(lower, bad) {
			return ErrThemeTokenValue
		}
	}
	return nil
}

func validateCSS(css string) error {
	if len(css) > MaxCSSBytes {
		return ErrThemeCSSSize
	}
	if css == "" {
		return nil
	}
	stripped := cssComment.ReplaceAllString(css, "")
	lower := strings.ToLower(stripped)
	for _, bad := range forbiddenCSSSubstrings {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("%s: %w", bad, ErrThemeCSSContent)
		}
	}
	depth := 0
	for _, r := range stripped {
		switch r {
		case '{':
			depth++
			if depth > maxCSSDepth {
				return ErrThemeCSSBraces
			}
		case '}':
			depth--
			if depth < 0 {
				return ErrThemeCSSBraces
			}
		}
	}
	if depth != 0 {
		return ErrThemeCSSBraces
	}
	return nil
}
