package install

import (
	"strings"
	"unicode/utf16"
)

// Inno пишет /SAVEINF в UTF-16LE с BOM; на диске также встречается UTF-8
// (например, если файл прошёл через сторонний инструмент). Обе кодировки
// нужно уметь читать без внешних зависимостей.
func decodeInfText(data []byte) string {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		payload := data[2:]
		units := make([]uint16, len(payload)/2)
		for i := range units {
			units[i] = uint16(payload[2*i]) | uint16(payload[2*i+1])<<8
		}
		return string(utf16.Decode(units))
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return string(data[3:])
	}
	return string(data)
}

func infComponents(data []byte) ([]string, bool) {
	text := decodeInfText(data)
	section := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if !strings.EqualFold(section, "Setup") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if !strings.EqualFold(key, "Components") {
			continue
		}
		value := strings.TrimSpace(line[idx+1:])
		if value == "" {
			return nil, true
		}
		return splitComponentList(value), true
	}
	return nil, false
}

func splitComponentList(value string) []string {
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func redistDict(opts installOptions) []string {
	if !opts.SkipShortcuts && !opts.SkipExtras {
		return nil
	}
	dict := make([]string, 0, len(shortcutTasks)+len(extraTasks)+2)
	if opts.SkipShortcuts {
		dict = append(dict, shortcutTasks...)
	}
	if opts.SkipExtras {
		dict = append(dict, extraTasks...)
		dict = append(dict, "redist", "prereq")
	}
	return dict
}

func componentDeclined(component string, dict []string) bool {
	for _, seg := range strings.Split(component, `\`) {
		seg = strings.ToLower(strings.TrimSpace(seg))
		if seg == "" {
			continue
		}
		for _, marker := range dict {
			m := strings.ToLower(marker)
			if len(m) <= 3 {
				if seg == m {
					return true
				}
				continue
			}
			if strings.Contains(seg, m) {
				return true
			}
		}
	}
	return false
}

func filterComponents(list []string, opts installOptions) ([]string, bool) {
	dict := redistDict(opts)
	if len(dict) == 0 || len(list) == 0 {
		return list, false
	}
	kept := make([]string, 0, len(list))
	removed := false
	for _, comp := range list {
		if componentDeclined(comp, dict) {
			removed = true
			continue
		}
		kept = append(kept, comp)
	}
	if !removed || len(kept) == 0 {
		return list, false
	}
	return kept, true
}
