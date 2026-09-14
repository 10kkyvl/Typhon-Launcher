package titles

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Feed packaging notes do not identify a different catalog game. Keep this
// cleanup in matching: Parse still extracts the release's versions/languages.
var reMatchExtras = regexp.MustCompile(`(?i)\s+\+\s*(?:(?:(?:all|\d+)\s+)?(?:bonus(?:es)?|dlcs?|osts?|soundtracks?|wallpapers)|windows\s+7\s+fix|essential\s+mods\s+and\s+fixes|radio\s+downgrader|vanilla\s+fixes\s+modpack|nve\s+(?:platinum\s+)?modpack)\b`)

var reMatchNumberPair = regexp.MustCompile(`\b(\d{1,2})\s*\(([IVX]+)\)`)

var matchNumerals = []string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII", "XIII", "XIV", "XV", "XVI", "XVII", "XVIII", "XIX", "XX"}

func matchNumeralValue(s string) int {
	for i, roman := range matchNumerals {
		if strings.EqualFold(s, roman) {
			return i + 1
		}
	}
	return 0
}

func withoutMatchExtras(raw string) string {
	depth, previous := 0, 0
	for _, loc := range reMatchExtras.FindAllStringIndex(raw, -1) {
		for _, r := range raw[previous:loc[0]] {
			switch r {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth = max(0, depth-1)
			}
		}
		if depth == 0 {
			return raw[:loc[0]]
		}
		previous = loc[0]
	}
	return raw
}

// MatchNames preserves edition identity and offers an English title embedded
// beside its Cyrillic translation. Arbitrary subtitles and sequel numbers stay.
func MatchNames(raw string) []string {
	names := matchNames(raw)
	repaired := repairMixedLatinWords(raw)
	if repaired == raw {
		return names
	}
	// Preserve real mixed-script catalog spellings too. Try both edition
	// spellings before either base-title fallback.
	alternatives := matchNames(repaired)
	out := []string{}
	seen := map[string]bool{}
	for i := range max(len(names), len(alternatives)) {
		for _, variants := range [][]string{names, alternatives} {
			if i < len(variants) && !seen[Normalize(variants[i])] {
				out = append(out, variants[i])
				seen[Normalize(variants[i])] = true
			}
		}
	}
	return out
}

func matchNames(raw string) []string {
	// Some feeds write the same sequel number twice: "2 (II)". Only
	// collapse a pair whose numeric values agree; "2 (III)" stays intact.
	raw = reMatchNumberPair.ReplaceAllStringFunc(raw, func(pair string) string {
		parts := reMatchNumberPair.FindStringSubmatch(pair)
		if n := matchNumeralValue(parts[2]); n > 0 && parts[1] == strconv.Itoa(n) {
			return parts[2]
		}
		return pair
	})
	p := Parse(withoutMatchExtras(raw))
	base := p.Base
	if strings.Contains(base, "/") {
		parts := strings.Split(base, "/")
		if len(parts) == 2 && hasCyrillic(parts[0]) && !hasCyrillic(parts[1]) {
			base = strings.TrimSpace(parts[1])
		}
	}
	if hasCyrillic(base) {
		for _, bracket := range reBracket.FindAllString(base, -1) {
			inner := strings.TrimSpace(bracket[1 : len(bracket)-1])
			if !hasCyrillic(inner) && matchNumeralValue(inner) == 0 && strings.IndexFunc(inner, unicode.IsLetter) >= 0 {
				base = inner
				break
			}
		}
	}
	// GTA feeds commonly put the expanded title beside its abbreviation.
	if parts := strings.Split(base, "/"); len(parts) == 2 {
		left, right := expandGTA(strings.TrimSpace(parts[0])), expandGTA(strings.TrimSpace(parts[1]))
		if strings.HasPrefix(strings.ToLower(left), "grand theft auto") {
			switch {
			case Normalize(left) == Normalize(right):
				base = left
			case strings.EqualFold(right, left+" (Legacy)"):
				base = left + " Legacy"
			case strings.EqualFold(left, right+" (Legacy)"):
				base = right + " Legacy"
			}
		}
	}
	base = expandGTA(base)
	out := []string{}
	if p.Edition != "" {
		out = append(out, strings.TrimSpace(base+" "+p.Edition))
	}
	identityEdition := false
	for _, word := range []string{"remaster", "enhanced", "definitive", "anniversary", "director"} {
		if strings.Contains(strings.ToLower(p.Edition), word) {
			identityEdition = true
		}
	}
	if base != "" && !identityEdition {
		out = append(out, base)
	}
	return out
}
func hasCyrillic(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) >= 0
}
func expandGTA(s string) string {
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "gta ") || strings.HasPrefix(lower, "gta:") {
		s = "Grand Theft Auto" + s[3:]
	}
	for _, pair := range [][2]string{{"Grand Theft Auto 3", "Grand Theft Auto III"}, {"Grand Theft Auto 4", "Grand Theft Auto IV"}, {"Grand Theft Auto 5", "Grand Theft Auto V"}} {
		if strings.EqualFold(s, pair[0]) {
			return pair[1]
		}
		if len(s) > len(pair[0]) && strings.EqualFold(s[:len(pair[0])], pair[0]) && strings.ContainsRune(" :(", rune(s[len(pair[0])])) {
			return pair[1] + s[len(pair[0]):]
		}
	}
	return s
}

// Repair lookalike Cyrillic letters only inside otherwise Latin words (Niоh,
// Dаys). Whole Russian words and mixed words with non-lookalike letters remain.
func repairMixedLatinWords(s string) string {
	var out strings.Builder
	var word []rune
	flush := func() {
		latin, cyrillic, repairable := false, false, true
		for _, r := range word {
			latin = latin || unicode.In(r, unicode.Latin)
			if unicode.In(r, unicode.Cyrillic) {
				cyrillic = true
				if _, ok := latinLookalikes[r]; !ok {
					repairable = false
				}
			}
		}
		for _, r := range word {
			if latin && cyrillic && repairable {
				if replacement, ok := latinLookalikes[r]; ok {
					r = replacement
				}
			}
			out.WriteRune(r)
		}
		word = word[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			word = append(word, r)
		} else {
			flush()
			out.WriteRune(r)
		}
	}
	flush()
	return out.String()
}

var latinLookalikes = map[rune]rune{
	'а': 'a', 'А': 'A', 'е': 'e', 'Е': 'E', 'о': 'o', 'О': 'O',
	'р': 'p', 'Р': 'P', 'с': 'c', 'С': 'C', 'х': 'x', 'Х': 'X',
	'у': 'y', 'У': 'Y', 'і': 'i', 'І': 'I', 'ј': 'j', 'Ј': 'J',
	'ѕ': 's', 'Ѕ': 'S', 'К': 'K', 'М': 'M', 'Т': 'T', 'В': 'B', 'Н': 'H',
}
