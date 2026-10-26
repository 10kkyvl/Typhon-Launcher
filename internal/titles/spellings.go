package titles

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSpellings     = 5000
	maxSpellingWords = 12
	maxSpellingTail  = 8
)

//go:embed titles-spellings.json
var builtinSpellings []byte

// Хвост названия, который не часть игры, а пометка раздачи: он либо
// переводится, либо отбрасывается в запасном варианте.
var spellingTailWords = map[string]string{
	"ремейк":      "Remake",
	"ремастер":    "Remastered",
	"мультиплеер": "Multiplayer",
	"multiplayer": "Multiplayer",
}

func validateSpellings(table map[string]string) error {
	if len(table) > MaxSpellings {
		return fmt.Errorf("%w: titleSpellings has %d entries, limit %d", ErrDictTooLarge, len(table), MaxSpellings)
	}
	for key, value := range table {
		if err := checkToken("titleSpellings", key); err != nil {
			return err
		}
		if value == "" {
			continue
		}
		if err := checkToken("titleSpellings", value); err != nil {
			return err
		}
		if len(strings.Fields(spellingKey(key))) > maxSpellingWords || spellingKey(key) == "" {
			return fmt.Errorf("%w: titleSpellings key %q has no usable words or too many", ErrDictEntry, key)
		}
		for _, variant := range strings.Split(value, "|") {
			if err := checkSpellingValue(variant); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSpellingValue(variant string) error {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return fmt.Errorf("%w: titleSpellings has a blank variant", ErrDictEntry)
	}
	if strings.Contains(variant, "://") {
		return fmt.Errorf("%w: titleSpellings variant looks like a link", ErrDictEntry)
	}
	for _, r := range variant {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: titleSpellings variant has a control character", ErrDictEntry)
		}
		if unicode.In(r, unicode.Cyrillic) {
			return fmt.Errorf("%w: titleSpellings variant must be the English title", ErrDictEntry)
		}
	}
	return nil
}

func spellingKey(s string) string {
	return strings.Join(spellingWords(s), " ")
}

func spellingWords(s string) []string {
	return strings.Fields(Normalize(strings.ReplaceAll(strings.ToLower(s), "ё", "е")))
}

// spellingNames превращает русское или фонетическое написание названия в
// английское. Совпадение — точное по самой длинной фразе в начале названия,
// без нечёткого сравнения: «Нид Фор Спид Прострит» нельзя спутать с Hot
// Pursuit. Номер, год и латинский хвост («3 Ремейк», «Death of the Outsider»)
// сохраняются; неизвестное русское слово в хвосте отменяет вариант.
func (d *Dict) spellingNames(names []string) []string {
	if d == nil || len(d.spellings) == 0 {
		return nil
	}
	var out []string
	for _, name := range names {
		if !hasCyrillic(name) {
			continue
		}
		out = append(out, d.spellingVariants(name)...)
	}
	return out
}

type spellingWord struct {
	norm string
	orig string
}

func (d *Dict) spellingVariants(name string) []string {
	fields := strings.Fields(strings.Map(func(r rune) rune {
		switch r {
		case '(', ')', '[', ']', '{', '}', ':', ',', ';', '!', '?', '+', '/', '|', '–', '—', '-', '.', '«', '»', '"':
			return ' '
		}
		return r
	}, name))
	var words []spellingWord
	for _, field := range fields {
		parts := spellingWords(field)
		for _, part := range parts {
			words = append(words, spellingWord{norm: part, orig: part})
		}
		if len(parts) == 1 {
			words[len(words)-1].orig = field
		}
	}
	if len(words) == 0 {
		return nil
	}
	for n := min(len(words), maxSpellingWords); n >= 1; n-- {
		key := make([]string, n)
		for i := range key {
			key[i] = words[i].norm
		}
		value, ok := d.spellings[strings.Join(key, " ")]
		if !ok {
			continue
		}
		return d.spellingTails(strings.Split(value, "|"), words[n:])
	}
	return nil
}

func (d *Dict) spellingTails(values []string, tail []spellingWord) []string {
	if len(tail) > maxSpellingTail {
		return nil
	}
	for _, w := range tail {
		if !d.spellingTailWordOK(w) {
			return nil
		}
	}
	type flags struct{ dropMapped, dropYear bool }
	order := []flags{{false, false}, {true, false}, {false, true}, {true, true}}
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, f := range order {
			parts := []string{value}
			for _, w := range tail {
				if mapped, ok := spellingTailWords[w.norm]; ok {
					if !f.dropMapped {
						parts = append(parts, mapped)
					}
					continue
				}
				if isYearToken(w.norm) {
					if !f.dropYear {
						parts = append(parts, w.norm)
					}
					continue
				}
				parts = append(parts, w.orig)
			}
			candidate := strings.Join(parts, " ")
			if key := Normalize(candidate); !seen[key] {
				seen[key] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

func withSpellings(names []string) []string {
	extra := Active().spellingNames(names)
	if len(extra) == 0 {
		return names
	}
	return limitMatchNames(append(append([]string(nil), names...), extra...))
}

func isYearToken(s string) bool {
	if len(s) != 4 || (s[:2] != "19" && s[:2] != "20") {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var reSpellingNumeral = regexp.MustCompile(`^(?:[0-9]+|i{1,3}|iv|vi{0,3}|ix|xi{0,3}|x{1,2})$`)

// Хвост пропускается в вариант только если он похож на часть названия: номер,
// римская цифра или обычное латинское слово. Служебные слова раздачи, коды
// языков и одиночные буквы означают, что разбор названия не очистил.
func (d *Dict) spellingTailWordOK(w spellingWord) bool {
	if _, ok := spellingTailWords[w.norm]; ok {
		return true
	}
	if isYearToken(w.norm) || reSpellingNumeral.MatchString(w.norm) {
		return true
	}
	if hasCyrillic(w.orig) || utf8.RuneCountInString(w.norm) < 2 {
		return false
	}
	if _, ok := d.releaseSingleTags[w.norm]; ok {
		return false
	}
	if _, ok := d.archTokens[w.norm]; ok {
		return false
	}
	return !d.isLangCode(w.norm)
}
