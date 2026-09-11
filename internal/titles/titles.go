package titles

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Parsed struct {
	Base       string
	Normalized string
	Edition    string
	Version    string
	RawVersion string
	Languages  []string
	Year       int
	Tags       []string
	DLCCount   int
}

func (d *Dict) Parse(raw string) Parsed {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Parsed{}
	}

	s = reExt.ReplaceAllString(s, "")
	s = reURL.ReplaceAllString(s, " ")
	s = reWWW.ReplaceAllString(s, " ")

	s, markerTags := d.extractMarkers(s)
	s, rawVersion, version := extractVersion(s)
	s, dlcCount := extractDLCCount(s)
	s, year, bracketLangs, bracketTags := d.extractBrackets(s)
	s, dashLangs, dashTags := d.extractLangAndDashTags(s)

	s = reDecimalDot.ReplaceAllString(s, "${1}\x00${2}")
	s = reSepRun.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "\x00", ".")
	s = reSpaceRun.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	var words []string
	if s != "" {
		words = strings.Fields(s)
	}

	words, edition, scanTags := d.trailingScan(words)

	base := strings.Join(words, " ")
	base = strings.Trim(base, " -,:;|\u2014\u2013")

	var languages []string
	languages = append(languages, bracketLangs...)
	languages = append(languages, dashLangs...)
	if len(languages) == 0 {
		languages = nil
	}

	var tags []string
	tags = append(tags, markerTags...)
	tags = append(tags, bracketTags...)
	tags = append(tags, dashTags...)
	tags = append(tags, scanTags...)
	tags = uniqueSortedLower(tags)

	return Parsed{
		Base:       base,
		Normalized: Normalize(base),
		Edition:    edition,
		Version:    version,
		RawVersion: rawVersion,
		Languages:  languages,
		Year:       year,
		Tags:       tags,
		DLCCount:   dlcCount,
	}
}

// extractMarkers снимает маркеры раздачи: то, что источник дописывает за «|»
// или за последним длинным тире — «Portable», «Архив», «P2P», «GOG»,
// «RePack от xatab». Сегмент выбрасывается только целиком опознанным: «| Season
// 1» в «A Rat's Quest | Season 1 — v1.0 | GOG» остаётся частью названия.
func (d *Dict) extractMarkers(s string) (string, []string) {
	var tags []string

	parts := strings.Split(s, "|")
	kept := parts[:1:1]
	for _, seg := range parts[1:] {
		found, ok := d.markerSegment(seg)
		if !ok {
			kept = append(kept, seg)
			continue
		}
		tags = append(tags, found...)
	}
	s = strings.Join(kept, "|")

	for _, dash := range []string{"\u2014", "\u2013"} {
		i := strings.LastIndex(s, dash)
		if i < 0 {
			continue
		}
		found, ok := d.markerSegment(s[i+len(dash):])
		if !ok {
			continue
		}
		tags = append(tags, found...)
		s = s[:i] + " "
	}

	return s, tags
}

// markerSegment опознаёт один сегмент маркера. Пустой сегмент — это висящий
// разделитель, и он тоже выбрасывается.
func (d *Dict) markerSegment(seg string) ([]string, bool) {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return nil, true
	}
	if m := reMarkerRepack.FindStringSubmatch(seg); m != nil {
		kind := "repack"
		if strings.Contains(strings.ToLower(m[1]), "rip") {
			kind = "steam-rip"
		}
		tags := []string{kind}
		if slug := d.repackerSlug(m[2]); slug != "" {
			tags = append(tags, slug)
		}
		return tags, true
	}
	if tag, ok := d.markerPhrase(Normalize(seg)); ok {
		return []string{tag}, true
	}
	return nil, false
}

func extractDLCCount(s string) (string, int) {
	loc := reDLCCount.FindStringSubmatchIndex(s)
	if loc == nil {
		return s, 0
	}
	n, err := strconv.Atoi(s[loc[2]:loc[3]])
	if err != nil {
		return s, 0
	}
	newS := s[:loc[0]] + " " + s[loc[1]:]
	return newS, n
}

func extractVersion(s string) (string, string, string) {
	patterns := []*regexp.Regexp{reBuildVer, reUpdateVer, rePatchVer, reHotfixVer, reVVer, reRVer}

	bestStart := -1
	var bestLoc []int
	for _, re := range patterns {
		loc := re.FindStringSubmatchIndex(s)
		if loc == nil {
			continue
		}
		if bestStart == -1 || loc[0] < bestStart {
			bestStart = loc[0]
			bestLoc = loc
		}
	}
	if bestLoc == nil {
		return s, "", ""
	}

	raw := s[bestLoc[0]:bestLoc[1]]
	ver := s[bestLoc[2]:bestLoc[3]]
	newS := s[:bestLoc[0]] + " " + s[bestLoc[1]:]
	return newS, strings.TrimSpace(raw), ver
}

func (d *Dict) extractBrackets(s string) (string, int, []string, []string) {
	year := 0
	var langs []string
	var tags []string

	result := reBracket.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(m[1 : len(m)-1])
		if inner == "" {
			return " "
		}
		if found, ok := d.bracketMarker(inner); ok {
			tags = append(tags, found...)
			return " "
		}
		// A service phrase can share a bracket with a known release
		// decorator, such as the language marker in
		// "+ Windows 7 Fix, MULTi6". Try the decorator-stripped form,
		// but only drop the bracket if what remains is still a dictionary
		// marker; arbitrary parenthesized title text stays intact below.
		cleanedInner, decoratorLangs, decoratorTags := d.extractLangAndDashTags(inner)
		if cleanedInner != inner {
			if found, ok := d.bracketMarker(cleanedInner); ok {
				langs = append(langs, decoratorLangs...)
				tags = append(tags, decoratorTags...)
				tags = append(tags, found...)
				return " "
			}
		}
		words := reBracketSplit.Split(inner, -1)
		var cleaned []string
		for _, w := range words {
			if w != "" {
				cleaned = append(cleaned, w)
			}
		}
		if len(cleaned) == 0 {
			return " "
		}

		knownAll := true
		skipNext := false
		var localLangs []string
		var localTags []string
		localYear := 0

		for _, w := range cleaned {
			if skipNext {
				skipNext = false
				continue
			}
			lw := strings.ToLower(w)
			switch {
			case reYear.MatchString(w):
				y, err := strconv.Atoi(w)
				if err == nil {
					localYear = y
				}
			case lw == "by":
				skipNext = true
			case d.isFiller(lw):
			case reMulti.MatchString(w) && reMulti.FindString(w) == w:
				localLangs = append(localLangs, w)
			case d.isLangCode(lw):
				localLangs = append(localLangs, strings.ToUpper(w))
			case d.archTokens[lw] != "":
				localTags = append(localTags, d.archTokens[lw])
			case d.releaseSingleTags[lw] != "":
				localTags = append(localTags, d.releaseSingleTags[lw])
			case lw == "rip" || lw == "steam":
				localTags = append(localTags, "steam-rip")
			default:
				knownAll = false
			}
		}

		if !knownAll {
			return m
		}
		if localYear != 0 {
			year = localYear
		}
		langs = append(langs, localLangs...)
		tags = append(tags, localTags...)
		return " "
	})

	return result, year, langs, tags
}

func (d *Dict) extractLangAndDashTags(s string) (string, []string, []string) {
	var langs []string
	var tags []string

	s = d.reLangCombo.ReplaceAllStringFunc(s, func(m string) string {
		parts := splitLangCombo(m)
		for _, p := range parts {
			langs = append(langs, strings.ToUpper(p))
		}
		return " "
	})
	s = reMulti.ReplaceAllStringFunc(s, func(m string) string {
		langs = append(langs, m)
		return " "
	})
	s = d.reLangSingle.ReplaceAllStringFunc(s, func(m string) string {
		langs = append(langs, strings.ToUpper(m))
		return " "
	})
	s = rePortable.ReplaceAllStringFunc(s, func(m string) string {
		tags = append(tags, "portable")
		return " "
	})
	s = reSteamRip.ReplaceAllStringFunc(s, func(m string) string {
		tags = append(tags, "steam-rip")
		return " "
	})
	s = reRepackBy.ReplaceAllStringFunc(s, func(m string) string {
		tags = append(tags, "repack")
		return " "
	})

	return s, langs, tags
}

func splitLangCombo(m string) []string {
	var out []string
	cur := strings.Builder{}
	for _, r := range m {
		if r == '/' || r == '-' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (d *Dict) trailingScan(words []string) ([]string, string, []string) {
	var edition string
	var tags []string

	for {
		matched := false
		maxLen := 5
		if len(words) < maxLen {
			maxLen = len(words)
		}
		for L := maxLen; L >= 1; L-- {
			tail := words[len(words)-L:]
			normTail := make([]string, L)
			for i, w := range tail {
				normTail[i] = normKey(w)
			}

			kind, ok := d.matchPhrase(normTail)
			if !ok && L == 1 {
				kind, ok = d.matchSingle(normTail[0])
			}
			if !ok {
				continue
			}

			remaining := words[:len(words)-L]
			if len(remaining) == 0 {
				continue
			}

			words = remaining
			if kind == "edition" {
				if edition == "" {
					edition = strings.Join(tail, " ")
				}
			} else if strings.HasPrefix(kind, "tag:") {
				tags = append(tags, strings.TrimPrefix(kind, "tag:"))
			}
			matched = true
			break
		}
		if !matched {
			break
		}
	}

	return words, edition, tags
}

func (d *Dict) matchPhrase(normTail []string) (string, bool) {
	for _, p := range d.phraseTable {
		if len(p.norm) != len(normTail) {
			continue
		}
		if equalSlices(p.norm, normTail) {
			return p.kind, true
		}
	}
	return "", false
}

func (d *Dict) matchSingle(w string) (string, bool) {
	if v, ok := d.archTokens[w]; ok {
		return "tag:" + v, true
	}
	if v, ok := d.releaseSingleTags[w]; ok {
		return "tag:" + v, true
	}
	return "", false
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func uniqueSortedLower(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(in))
	for _, t := range in {
		set[strings.ToLower(t)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var diacriticMap = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y', 'ÿ': 'y',
}

func stripDiacritics(s string) string {
	return strings.Map(func(r rune) rune {
		if m, ok := diacriticMap[r]; ok {
			return m
		}
		return r
	}, s)
}

func Normalize(s string) string {
	s = strings.ToLower(s)
	s = stripDiacritics(s)

	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\'' || r == '’' || r == '`':
			continue
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}

	out := reSpaceRun.ReplaceAllString(b.String(), " ")
	return strings.TrimSpace(out)
}
