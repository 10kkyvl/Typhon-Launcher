package shortcut

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode"
)

type Link struct {
	Target      string
	Args        string
	WorkDir     string
	Icon        string
	Description string
}

const maxNameRunes = 100

const forbiddenChars = `<>:"/\|?*`

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// Remove удаляет файл ярлыка. Отсутствие файла уже является нужным конечным
// состоянием, поэтому fs.ErrNotExist ошибкой не считается.
func Remove(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("shortcut: удаление %s: %w", path, err)
}

func FileName(title string) (string, error) {
	base, err := sanitizeBaseName(title)
	if err != nil {
		return "", err
	}
	return base + ".lnk", nil
}

func sanitizeBaseName(title string) (string, error) {
	var b strings.Builder
	lastSpace := false
	for _, r := range title {
		switch {
		case r < 0x20:
			continue
		case strings.ContainsRune(forbiddenChars, r):
			continue
		case unicode.IsSpace(r):
			if lastSpace {
				continue
			}
			lastSpace = true
			b.WriteRune(' ')
		default:
			lastSpace = false
			b.WriteRune(r)
		}
	}

	name := strings.Trim(b.String(), " .")
	name = truncateRunes(name, maxNameRunes)
	name = strings.Trim(name, " .")
	if name == "" {
		return "", fmt.Errorf("shortcut: имя %q превращается в пустую строку после санитизации", title)
	}
	if isReservedName(name) {
		name += "_"
	}
	return name, nil
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

func isReservedName(name string) bool {
	stem := name
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		stem = name[:idx]
	}
	return reservedNames[strings.ToUpper(stem)]
}
