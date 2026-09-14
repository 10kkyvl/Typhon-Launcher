package titles

import (
	"strings"
	"unicode"
)

// MatchNames preserves edition identity and offers an English title embedded
// beside its Cyrillic translation. Arbitrary subtitles and sequel numbers stay.
func MatchNames(raw string) []string {
	p := Parse(raw)
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
	if parts := strings.Split(base, " / "); len(parts) == 2 && strings.HasPrefix(strings.ToLower(parts[0]), "grand theft auto") && strings.HasPrefix(strings.ToLower(parts[1]), "gta ") {
		base = parts[0]
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
