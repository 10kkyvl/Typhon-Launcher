package install

import (
	"strconv"
	"strings"
	"unicode"
)

const (
	entryLabelRunes   = 96
	progressNameRunes = 256
)

// Имена записей и вывод распаковщика приходят извне, а доходят до интерфейса:
// управляющие символы и перестановки направления заменяются, длина ограничена
// (инвариант 32).
func sanitizeText(s string, maxRunes int) string {
	runes := []rune(strings.ToValidUTF8(s, "?"))
	for i, r := range runes {
		if r != ' ' && !unicode.IsPrint(r) {
			runes[i] = '?'
		}
	}
	if len(runes) > maxRunes {
		runes = append(runes[:maxRunes], '…')
	}
	return string(runes)
}

func entryLabel(name string) string {
	return strconv.Quote(sanitizeText(name, entryLabelRunes))
}
