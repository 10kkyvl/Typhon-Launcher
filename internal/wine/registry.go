package wine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// UninstallEntry — запись «Установка и удаление программ» из реестра бутыля.
type UninstallEntry struct {
	Key             string
	DisplayName     string
	Command         string
	QuietCommand    string
	InstallLocation string
	ProductCode     string
	SystemComponent bool
}

// uninstallPrefixes: 32-битный установщик пишет в Wow6432Node, 64-битный — в
// основную ветку, и в одном бутыле встречаются обе.
var uninstallPrefixes = []string{
	`Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\`,
	`Software\\Wow6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\`,
}

// parseUninstall читает текстовый system.reg wine. Формат простой: строка
// [Ключ] открывает секцию, дальше "Имя"="Значение" до следующей секции.
func parseUninstall(text string) []UninstallEntry {
	out := make([]UninstallEntry, 0, 8)
	var current *UninstallEntry
	flush := func() {
		if current != nil {
			out = append(out, *current)
			current = nil
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "[") {
			flush()
			if key, ok := uninstallKey(line); ok {
				current = &UninstallEntry{Key: key}
			}
			continue
		}
		if current == nil || !strings.HasPrefix(line, `"`) {
			continue
		}
		name, value, ok := regValue(line)
		if !ok {
			continue
		}
		switch strings.ToLower(name) {
		case "displayname":
			current.DisplayName = value
		case "uninstallstring":
			current.Command = value
		case "quietuninstallstring":
			current.QuietCommand = value
		case "installlocation":
			current.InstallLocation = value
		case "productcode":
			current.ProductCode = value
		case "systemcomponent":
			current.SystemComponent = value != "0"
		}
	}
	flush()
	return out
}

func uninstallKey(line string) (string, bool) {
	end := strings.Index(line, "]")
	if end < 0 {
		return "", false
	}
	full := line[1:end]
	for _, prefix := range uninstallPrefixes {
		if rest, found := strings.CutPrefix(full, prefix); found {
			return unescapeReg(rest), true
		}
	}
	return "", false
}

// regValue разбирает строку вида "Имя"="Значение" или "Имя"=dword:00000001.
func regValue(line string) (name, value string, ok bool) {
	closing := strings.Index(line[1:], `"`)
	if closing < 0 {
		return "", "", false
	}
	name = line[1 : closing+1]
	rest, found := strings.CutPrefix(line[closing+2:], "=")
	if !found {
		return "", "", false
	}
	if quoted, isQuoted := strings.CutPrefix(rest, `"`); isQuoted {
		return name, unescapeReg(strings.TrimSuffix(quoted, `"`)), true
	}
	if hex, isDword := strings.CutPrefix(rest, "dword:"); isDword {
		parsed, err := strconv.ParseUint(strings.TrimSpace(hex), 16, 32)
		if err != nil {
			return "", "", false
		}
		return name, strconv.FormatUint(parsed, 10), true
	}
	return name, rest, true
}

// unescapeReg снимает удвоение обратных слэшей и экранирование кавычек,
// которым wine кодирует значения в system.reg.
var regUnescaper = strings.NewReplacer(`\\`, `\`, `\"`, `"`)

func unescapeReg(value string) string { return regUnescaper.Replace(value) }

// UninstallEntries читает реестр бутыля. Отсутствие файла не ошибка: бутыль
// мог быть создан, но ещё ни разу не запущен.
func (b Bottle) UninstallEntries() ([]UninstallEntry, error) {
	data, err := os.ReadFile(filepath.Join(b.Path, "system.reg"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("чтение реестра бутыля %s: %w", b.Name, err)
	}
	return parseUninstall(string(data)), nil
}
