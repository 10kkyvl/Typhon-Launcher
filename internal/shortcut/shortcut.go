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
	// AppUserModelID привязывает ярлык и запущенный им процесс к одной
	// группе на панели задач. Пусто — идентификатор не пишется. Только Windows.
	AppUserModelID string
}

const (
	LauncherAppID   = "Typhon.Launcher"
	gameAppIDPrefix = "Typhon.Game."
	maxAppIDLen     = 128
)

var (
	ErrAppIDEmpty   = errors.New("shortcut: пустой AppUserModelID")
	ErrAppIDTooLong = errors.New("shortcut: AppUserModelID длиннее 128 символов")
	ErrAppIDInvalid = errors.New("shortcut: недопустимый символ в AppUserModelID")
)

// IsPlayArgs сообщает, что аргументы ярлыка запускают игру через --play.
func IsPlayArgs(args string) bool { return strings.Contains(args, "--play") }

var (
	ErrBadGameID    = errors.New("shortcut: идентификатор игры непригоден для командной строки")
	ErrBadProfileID = errors.New("shortcut: идентификатор профиля непригоден для командной строки")
)

// SafeCommandLineID отвергает идентификаторы, которые в аргументах ярлыка
// разъехались бы на несколько аргументов или изменили смысл командной строки.
func SafeCommandLineID(id string) bool {
	if id == "" {
		return false
	}
	// Идентификатор, начинающийся с дефиса, разбирается как ещё один флаг
	// командной строки, а не как значение --play.
	if strings.HasPrefix(id, "-") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// PlayArgs — единственное место, где собирается командная строка запуска игры
// из ярлыка, Steam и Jump List; main.playTarget разбирает именно этот формат.
func PlayArgs(gameID, profileID string) (string, error) {
	if !SafeCommandLineID(gameID) {
		return "", fmt.Errorf("%w: %q", ErrBadGameID, gameID)
	}
	args := "--play " + gameID
	if profileID == "" {
		return args, nil
	}
	if !SafeCommandLineID(profileID) {
		return "", fmt.Errorf("%w: %q", ErrBadProfileID, profileID)
	}
	return args + " --profile " + profileID, nil
}

func appIDRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.', r == '-', r == '_':
		return true
	}
	return false
}

// ValidateAppID проверяет идентификатор по правилам оболочки: до 128 символов,
// без пробелов; здесь допускаются только ASCII-буквы, цифры, точка, дефис и
// подчёркивание.
func ValidateAppID(id string) error {
	if id == "" {
		return ErrAppIDEmpty
	}
	if len(id) > maxAppIDLen {
		return fmt.Errorf("%w: %d", ErrAppIDTooLong, len(id))
	}
	for _, r := range id {
		if !appIDRune(r) {
			return fmt.Errorf("%w: %q", ErrAppIDInvalid, r)
		}
	}
	return nil
}

// GameAppID строит идентификатор игры: недопустимые символы заменяются на
// подчёркивание, длина обрезается до 128.
func GameAppID(gameID string) (string, error) {
	if gameID == "" {
		return "", ErrAppIDEmpty
	}
	var b strings.Builder
	b.WriteString(gameAppIDPrefix)
	for _, r := range gameID {
		if appIDRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	id := b.String()
	if len(id) > maxAppIDLen {
		id = id[:maxAppIDLen]
	}
	return id, nil
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

// Remove удаляет ярлык. Отсутствие пути уже является нужным конечным
// состоянием, поэтому fs.ErrNotExist ошибкой не считается.
//
// Lstat, а не Stat: ярлык на диске может быть как файлом (.lnk на Windows),
// так и каталогом-бандлом (.app на macOS). Если это каталог — удаляем
// рекурсивно; если обычный файл или симлинк — удаляем сам путь, не переходя
// по симлинку, иначе RemoveAll на символической ссылке снёс бы содержимое
// каталога, на который она указывает, а не саму ссылку.
func Remove(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("shortcut: удаление %s: %w", path, err)
	}
	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("shortcut: удаление %s: %w", path, err)
		}
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("shortcut: удаление %s: %w", path, err)
	}
	return nil
}

func FileName(title string) (string, error) {
	base, err := sanitizeBaseName(title)
	if err != nil {
		return "", err
	}
	return base + shortcutExt, nil
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
