package titles

import (
	"regexp"
	"strings"
	"unicode"
)

// Feed packaging notes do not identify a different catalog game. Keep this
// cleanup in matching: Parse still extracts the release's versions/languages.
var reMatchExtras = regexp.MustCompile(`(?i)\s+\+\s*(?:(?:(?:all|\d+)\s+)?(?:bonus(?:es)?|dlcs?|osts?|soundtracks?|wallpapers)|windows\s+7\s+fix|essential\s+mods\s+and\s+fixes)\b`)

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
			if !hasCyrillic(inner) && strings.IndexFunc(inner, unicode.IsLetter) >= 0 {
				base = inner
				break
			}
		}
	}
	// GTA feeds commonly put the expanded title beside its abbreviation.
	if parts := strings.Split(base, "/"); len(parts) == 2 {
		left, right := expandGTA(strings.TrimSpace(parts[0])), expandGTA(strings.TrimSpace(parts[1]))
		if strings.HasPrefix(strings.ToLower(left), "grand theft auto") && Normalize(left) == Normalize(right) {
			base = left
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
	}
	return s
}
