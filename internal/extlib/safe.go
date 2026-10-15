package extlib

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxTitleRunes = 200
	maxFileBytes  = 4 << 20
)

var errTooLarge = errors.New("файл слишком большой")

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // G703: путь собран из корня лаунчера (реестр/ProgramData) и имени записи каталога, размер ограничен maxFileBytes (инварианты 32, 34)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%s: %w", path, errTooLarge)
	}
	return data, nil
}

func cleanTitle(raw string) string {
	var b strings.Builder
	space := false
	for _, r := range raw {
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	out := []rune(b.String())
	if len(out) > maxTitleRunes {
		out = out[:maxTitleRunes]
	}
	return strings.TrimSpace(string(out))
}

func hasDotDot(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

func checkDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("путь не задан")
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("путь содержит нулевой символ")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("путь не абсолютный: %s", path)
	}
	if hasDotDot(path) {
		return "", fmt.Errorf("путь содержит '..': %s", path)
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("каталог недоступен: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("не каталог: %s", clean)
	}
	return clean, nil
}

func isDigits(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
