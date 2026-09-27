package titles

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Feed packaging notes do not identify a different catalog game. Keep this
// cleanup in matching: Parse still extracts the release's versions/languages.
// The "+" and its count/"all" prefix are sometimes glued straight onto the
// version with no space ("v1.3.9+DLC", "v99i2+allDLC"), so both spaces are
// optional, not required.
var reMatchExtras = regexp.MustCompile(`(?i)\s*\+\s*(?:(?:all\s*|\d+\s*)?(?:bonus(?:es)?|dlcs?|osts?|soundtracks?|wallpapers)|windows\s+7\s+fix|essential\s+mods\s+and\s+fixes|radio\s+downgrader|vanilla\s+fixes\s+modpack|nve\s+(?:platinum\s+)?modpack)\b`)

var (
	reMatchNumberPair      = regexp.MustCompile(`\b(\d{1,2})\s*\(([IVX]+)\)`)
	reMatchRomanNumberPair = regexp.MustCompile(`\b([IVX]+)\s*\((\d{1,2})\)`)
	reMatchAcronymPair     = regexp.MustCompile(`(?i)((?:[a-z]\.){2,}[a-z])\s*\(([a-z]{2,})\)`)
)

// A commercial package can follow an identity-bearing edition. Keep the
// remaster in fallback names even though Parse exposes the outer package.
var reMatchLayeredEdition = regexp.MustCompile(`(?i)\b((?:remastered|remaster|enhanced|definitive|anniversary)(?:\s+edition)?|director['’]s\s+cut)\s+((?:digital\s+)?(?:deluxe|ultimate|gold|premium|complete|collector['’]?s|special)\s+edition)\b`)

// Некоторые фиды кладут номер сборки в отдельную скобку, не помечая его
// словом Build: «Monster Train 2 (14193)» или «StarRupture
// (0.1.1.112941-S)». Числовая скобка становится метаданными только рядом с
// явным маркером раздачи; так обычный «Game 2 (III)» и настоящий подзаголовок
// не исчезают из имени.
var (
	reMatchReleaseBracket      = regexp.MustCompile(`(?i)^\s*v(?:[.\s]+(?:build|patch|update|hotfix)\b|[.\s]+necro\s+patch\b)`)
	reMatchBuildIDBracket      = regexp.MustCompile(`(?i)^(?:\d{5,}(?:[._-][0-9a-z]+)*|(?:\d+[._-]){2,}[0-9a-z]+(?:[._-][0-9a-z]+)*|[0-9a-f]{6,}|(?:alpha|beta)\s+\d+(?:[._-][0-9a-z]+)+|\d{4}[-/.]\d{1,2}[-/.]\d{1,2}(?:[-/.][0-9a-z]+)*(?:\s+[a-z]+\d+)?)$`)
	reMatchVersionPatchBracket = regexp.MustCompile(`(?i)^\d+(?:\.\d+)+\s+(?:patch|hotfix|update)\s+\d+$`)
	// A release bracket can contain a feed label before its version, for
	// example "GOG v50507" or "BuildID 7368608". Remove it only when a
	// source marker follows the bracket, so title brackets such as "(All
	// Stars)" remain intact.
	reMatchReleasePayloadBracket = regexp.MustCompile(`(?i)\b(?:v(?:er)?[.\s]*\d+|build(?:id)?[.#\-_ ]*\d+|(?:update|patch|hotfix)[.#\-_ ]*\d+)\b`)
)

func withoutMatchBuildBrackets(raw string) string {
	for {
		locations := reBracket.FindAllStringIndex(raw, -1)
		if len(locations) == 0 {
			return raw
		}
		var out strings.Builder
		last := 0
		changed := false
		for _, loc := range locations {
			out.WriteString(raw[last:loc[0]])
			bracket := raw[loc[0]:loc[1]]
			inner := strings.TrimSpace(bracket[1 : len(bracket)-1])
			remove := reMatchReleaseBracket.MatchString(inner)
			if !remove && (reMatchBuildIDBracket.MatchString(inner) || reMatchVersionPatchBracket.MatchString(inner)) {
				remove = matchReleaseMarkerAfter(raw[loc[1]:]) || matchReleasePrefixBefore(raw, loc[0])
			}
			if !remove && matchReleasePayloadBracket(inner) {
				remove = matchReleaseMarkerAfter(raw[loc[1]:])
			}
			if remove {
				out.WriteByte(' ')
				changed = true
			} else {
				out.WriteString(bracket)
			}
			last = loc[1]
		}
		out.WriteString(raw[last:])
		if !changed {
			return raw
		}
		raw = out.String()
	}
}

// matchReleasePrefixBefore reports whether a flat bracket is nested directly
// in a release bracket, such as the build number in
// "(v 1.6.0 hotfix(40765)+DLC)". The inner bracket is removed first; the
// next pass can then recognize the now-flat outer release bracket.
func matchReleasePrefixBefore(raw string, start int) bool {
	stack := []int{}
	for i := 0; i < start; i++ {
		switch raw[i] {
		case '(', '[', '{':
			stack = append(stack, i)
		case ')', ']', '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) == 0 {
		return false
	}
	prefix := strings.TrimSpace(raw[stack[len(stack)-1]+1 : start])
	return matchReleasePayloadBracket(prefix)
}

// matchReleasePayloadBracket rejects a capital-V sequel token such as the
// English title in "Снайпер Элит 2 (Sniper Elite V2)". The same bracket
// matcher still accepts real release payloads such as "GOG v50507" and
// "BuildID 7368608".
func matchReleasePayloadBracket(inner string) bool {
	if !reMatchReleasePayloadBracket.MatchString(inner) {
		return false
	}
	return versionLocation(inner) != nil
}

func matchReleaseMarkerAfter(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range []string{"папка игры", "архив", "early access", "repack", "steam-rip", "gog", "p2p", "portable", "fitgirl", "dodi", "codex", "|"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// Всё, что фид дописывает за версией через «+», — состав раздачи: счётчики DLC,
// бонусы, эмуляторы, выделенный сервер, фиксы. Игру это не меняет, а точному
// сравнению имён в каталоге мешает. «+» до версии остаётся: он бывает частью
// названия издания.
func withoutPackagingTail(raw string) string {
	version := versionLocation(raw)
	if version == nil {
		return raw
	}
	depth := 0
	for i := range len(raw) {
		switch raw[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth = max(0, depth-1)
		case '+':
			// A release package can be enclosed in a malformed bracket, for
			// example "(v1.0 + Bonus Content ...]". Recognize only the same
			// closed list of package markers used by reMatchExtras; arbitrary
			// "+ Other Game" subtitles remain untouched.
			packagePlus := reMatchExtras.MatchString(" " + raw[i:])
			if (depth > 0 && !packagePlus) || i == 0 || raw[i-1] != ' ' {
				continue
			}
			rest := strings.TrimLeft(raw[i+1:], " ")
			if i >= version[1] || len(raw)-len(rest) == version[0] {
				return raw[:i]
			}
		}
	}
	return raw
}

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
		return limitMatchNames(names)
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
	// Каталог принимает не больше шести написаний на раздачу.
	return limitMatchNames(out)
}

const maxMatchNames = 6

func limitMatchNames(names []string) []string {
	out := make([]string, 0, min(len(names), maxMatchNames))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		key := Normalize(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
		if len(out) == maxMatchNames {
			break
		}
	}
	return out
}

func matchNames(raw string) []string {
	// Repeated sequel numbers are common in feed titles. Collapse only exact
	// Arabic/Roman pairs and dotted acronym spellings; a mismatching number or
	// an ordinary parenthesized subtitle remains part of the name.
	raw = reMatchRomanNumberPair.ReplaceAllStringFunc(raw, func(pair string) string {
		parts := reMatchRomanNumberPair.FindStringSubmatch(pair)
		if n := matchNumeralValue(parts[1]); n > 0 && parts[2] == strconv.Itoa(n) {
			return parts[1]
		}
		return pair
	})
	raw = reMatchAcronymPair.ReplaceAllStringFunc(raw, func(pair string) string {
		parts := reMatchAcronymPair.FindStringSubmatch(pair)
		if strings.EqualFold(strings.ReplaceAll(parts[1], ".", ""), parts[2]) {
			return parts[1]
		}
		return pair
	})
	// Some feeds write the same sequel number twice: "2 (II)". Only
	// collapse a pair whose numeric values agree; "2 (III)" stays intact.
	raw = reMatchNumberPair.ReplaceAllStringFunc(raw, func(pair string) string {
		parts := reMatchNumberPair.FindStringSubmatch(pair)
		if n := matchNumeralValue(parts[2]); n > 0 && parts[1] == strconv.Itoa(n) {
			return parts[2]
		}
		return pair
	})
	raw = withoutMatchBuildBrackets(raw)
	cleaned := withoutMatchExtras(withoutPackagingTail(raw))
	p := Parse(cleaned)
	layeredEdition := ""
	if parts := reMatchLayeredEdition.FindStringSubmatch(cleaned); parts != nil && strings.EqualFold(p.Edition, parts[2]) {
		inner := Parse(strings.Replace(cleaned, parts[0], parts[1], 1))
		// Bare "Enhanced" can already belong to the base title (Seven).
		// Reconstruct only editions that Parse actually stripped from it.
		if inner.Edition != "" {
			layeredEdition, p = parts[0], inner
		}
	}
	base := p.Base
	// A malformed feed title can leave the opening half of a release bracket
	// after version extraction, e.g. "Crimson Tactics (v1.0.0b + Bonus ...]".
	// Only trim unmatched opening punctuation at the end; balanced title
	// brackets and their subtitles remain untouched.
	base = strings.TrimSpace(strings.TrimRight(base, "([{"))
	if strings.Contains(base, "/") {
		parts := strings.Split(base, "/")
		if len(parts) == 2 && hasCyrillic(parts[0]) != hasCyrillic(parts[1]) {
			if hasCyrillic(parts[0]) {
				base = strings.TrimSpace(parts[1])
			} else {
				base = strings.TrimSpace(parts[0])
			}
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
	if layeredEdition != "" {
		out = append(out, strings.TrimSpace(base+" "+layeredEdition))
	}
	if p.Edition != "" {
		out = append(out, strings.TrimSpace(base+" "+p.Edition))
	}
	identityEdition := identityMatchEdition(p.Edition)
	if base != "" && !identityEdition {
		out = append(out, base)
	}
	if !identityEdition {
		out = append(out, releaseFallbackNames(base, cleaned)...)
	}
	return out
}

func identityMatchEdition(edition string) bool {
	for _, word := range []string{"remaster", "enhanced", "definitive", "anniversary", "director"} {
		if strings.Contains(strings.ToLower(edition), word) {
			return true
		}
	}
	return false
}

var (
	// A final package word is a useful source fallback only when a source has
	// put a separator before the package label.  The label itself may be named
	// ("Soundtrack Bundle", "Community of Planets Edition"): trimming just a
	// fixed list of adjectives loses the real base title in those releases.
	reEditionTail = regexp.MustCompile(`(?i)^(.*\S)\s*[:,–—-]\s+\S.*\b(edition|bundle|pack|collection|anthology)$`)
	// These are commercial package labels when they are appended as a final
	// word or phrase. Keep the complete spelling first and add this shorter
	// form only as a matching fallback. Remastered/master stay in the captured
	// identity: "Game Remastered Collection" must not become "Game".
	reCommercialTail = regexp.MustCompile(`(?i)^(.*?)\s+(?:(?:complete|ultimate|deluxe|gold|premium|special|supporter|launch|platinum|collector['’]?s)\s+pack|(?:(?:complete|ultimate|gold|premium|special|supporter|launch|collector['’]?s)\s+)?bundle)(?:\s+edition)?$`)
	// These package names are sufficiently closed that keeping the title
	// segment before the suffix is safer than the broad colon fallback.  In
	// particular, "The Run Limited Edition" must retain "The Run".
	rePreciseEditionTail  = regexp.MustCompile(`(?i)^(.*?\S)\s+(?:digital\s+)?(?:limited|deluxe|ultimate|gold|premium|special|supporter|launch|collector['’]?s|anniversary|enhanced|definitive|director['’]s|founder['’]?s|founders|year\s+(?:one|1|two|2)|day\s+(?:one|1|two|2)|goty|soundtrack)\s+edition$`)
	reRawCommercialTail   = regexp.MustCompile(`(?is)^\s*(.*?)\b(?:edition|bundle|pack|collection|anthology)\b`)
	reCommercialPackLabel = regexp.MustCompile(`(?i)^(?:complete|ultimate|deluxe|gold|premium|special|supporter|launch|platinum|collector['’]?s)(?:\s+edition)?$`)
	reCommercialMarker    = regexp.MustCompile(`(?i)\b(?:complete|limited|deluxe|ultimate|gold|premium|special|supporter|launch|collector['’]?s|anniversary|enhanced|definitive|director['’]s|founder['’]?s|founders|goty|soundtrack|ost|book|premiere|hunter['’]?s|aspiration|year\s+(?:one|1|two|2)|day\s+(?:one|1|two|2)|official|game|set|family|mega|master)\b`)
	// Publisher/developer brackets are feed metadata in a recurring legacy
	// source format: "Game (Ubisoft Entertainment)". Keep the full spelling as
	// the first candidate and offer the title before a bracket only for a
	// recognizable company marker.
	rePublisherBracket = regexp.MustCompile(`(?i)\b(?:activision|ubisoft|bethesda|sega|capcom|nintendo|konami|square\s+enix|2k|thq|tinybuild|frogwares|astragon|lince\s+works|phantom\s+8|studio\s+mdhr|inc\.?|llc|gmbh|ltd\.?)\b`)
	// An English title may legitimately end in "By" ("Stand By"). Known
	// repackers are already removed by Parse, so this fallback is limited to
	// the orphaned Russian attribution marker.
	reTrailingAttributionTitle = regexp.MustCompile(`(?i)^(.*\S)\s+от$`)
)

// releaseFallbackNames отдаёт запасные написания для точного сравнения с
// каталогом. Фид дописывает к названию перевод в скобке — «The Plucky Squire
// (Отважный Паж)» — и имя сборки издания, которого у игры в каталоге нет:
// «The Planet Crafter: Deluxe Bundle». Точное написание всё равно пробуется
// первым, так что игра, которая действительно так называется, не теряется.
func releaseFallbackNames(base, raw string) []string {
	var out []string
	if m := reTrailingAttributionTitle.FindStringSubmatch(base); m != nil {
		out = append(out, m[1])
		base = m[1]
	}
	if m := reBracket.FindStringIndex(base); m != nil && m[1] == len(base) {
		inner := base[m[0]+1 : m[1]-1]
		head := strings.TrimSpace(base[:m[0]])
		if head != "" && hasCyrillic(inner) != hasCyrillic(head) && matchNumeralValue(strings.TrimSpace(inner)) == 0 {
			out = append(out, head)
			base = head
		}
	}
	if m := reBracket.FindStringIndex(base); m != nil && m[1] == len(base) {
		inner := strings.TrimSpace(base[m[0]+1 : m[1]-1])
		head := strings.TrimSpace(base[:m[0]])
		if head != "" && rePublisherBracket.MatchString(inner) {
			out = append(out, head)
			base = head
			// The publisher can hide an edition from the initial Parse pass.
			// Removing that metadata must not expose the original-game fallback.
			if edition := Parse(head).Edition; identityMatchEdition(edition) {
				return out
			}
		}
	}
	// A closed commercial suffix is more precise than the old broad
	// colon-cut. Do not apply it when the source has an explicit separator
	// before that suffix: in "Hitman: ... - GOTY Edition" the text after the
	// colon is still a release label, while "Need for Speed: The Run Limited
	// Edition" has no such separator and must retain The Run.
	precise := ""
	if m := rePreciseEditionTail.FindStringSubmatch(base); m != nil && (!rawHasCommercialSeparator(raw) || !reEditionTail.MatchString(base)) && preciseCandidateAllowed(m[1]) {
		precise = normalizeFallbackCandidate(m[1])
		out = appendFallbackName(out, precise)
	}

	if m := reEditionTail.FindStringSubmatch(base); m != nil {
		if !commercialTailAllowed(base, m[1], m[2]) {
			m = nil
		}
		if m == nil {
			// A collection/anthology with an unrecognized label deliberately
			// keeps its complete title; do not recover the raw separator below.
		} else {
			candidate := m[1]
			// Parse removes an ASCII hyphen as punctuation. When a short, numeric
			// title head precedes a raw hyphen, recover the first complete title
			// segment instead of offering an ambiguous alias such as X4.
			if rawCandidate := rawShortHeadFallback(base, raw); rawCandidate != "" && !rawCandidateShorter(rawCandidate, candidate) {
				candidate = rawCandidate
			} else if rawCandidate := rawPreciseEditionFallback(raw); rawCandidate != "" && preciseCandidateAllowed(rawCandidate) && !rawCandidateShorter(rawCandidate, candidate) {
				// A raw separator can hide an identity-bearing subtitle from the
				// parsed colon fallback: "Need for Speed: Most Wanted - Limited
				// Edition" must retain Most Wanted.
				candidate = rawCandidate
			} else if rawCandidate := rawDigitalEditionFallback(raw); rawCandidate != "" && !rawCandidateShorter(rawCandidate, candidate) {
				// The same normalization issue affects the common
				// "Game: Subtitle - Digital Deluxe Edition" form. Keep the
				// subtitle before that explicit package separator.
				candidate = rawCandidate
			}
			if precise == "" {
				out = appendFallbackName(out, normalizeFallbackCandidate(candidate))
			}
		}
	}

	// Pack and bundle are also common without a title separator, e.g.
	// "Some Game Ultimate Pack". Keep this narrow fallback for those two
	// package words; Collection/Anthology stay behind the explicit-separator
	// guard above because they are often part of the real title.
	if m := reCommercialTail.FindStringSubmatch(base); m != nil {
		out = appendFallbackName(out, normalizeFallbackCandidate(m[1]))
	}
	return out
}

func commercialTailAllowed(base, head, suffix string) bool {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	label := commercialLabel(base, head, suffix)
	if suffix != "collection" && suffix != "anthology" && suffix != "pack" {
		// Edition and Bundle retain the historical named-package fallback.
		return true
	}

	if suffix == "pack" {
		return reCommercialPackLabel.MatchString(label)
	}
	// Collection and anthology are identity-bearing title words much more
	// often than Pack/Bundle. Require an explicit commercial descriptor after
	// the source separator; this accepts Complete/Premiere/OST collections but
	// leaves "The Cowabunga Collection" untouched.
	return reCommercialMarker.MatchString(label)
}

func preciseCandidateAllowed(candidate string) bool {
	colon := strings.Index(candidate, ":")
	if colon < 0 {
		return true
	}
	// "Dinkum: Official Soundtrack Edition" has a package descriptor
	// after the title colon. Prefer the generic title root in that shape;
	// the precise suffix must retain a real title subtitle such as The Run.
	return !reCommercialMarker.MatchString(candidate[colon+1:])
}

func commercialLabel(base, head, suffix string) string {
	if len(base) < len(head) {
		return ""
	}
	label := strings.TrimSpace(strings.Trim(base[len(head):], " :–—-"))
	fields := strings.Fields(label)
	if len(fields) > 0 && strings.EqualFold(fields[len(fields)-1], suffix) {
		label = strings.Join(fields[:len(fields)-1], " ")
	}
	return strings.TrimSpace(label)
}

func rawCandidateShorter(candidate, head string) bool {
	return len(strings.Fields(candidate)) < len(strings.Fields(head))
}

func normalizeFallbackCandidate(candidate string) string {
	candidate = strings.Trim(candidate, " \t,:–—-")
	if candidate == "" {
		return ""
	}
	parsed := Parse(candidate)
	if parsed.Base == "" {
		return candidate
	}
	if parsed.Edition != "" {
		return strings.TrimSpace(parsed.Base + " " + parsed.Edition)
	}
	return parsed.Base
}

// rawHasCommercialSeparator detects a source separator before the package
// segment even when Parse has already normalized an ASCII hyphen away.
func rawHasCommercialSeparator(raw string) bool {
	for _, sep := range []string{" - ", " – ", " — "} {
		at := strings.LastIndex(raw, sep)
		if at < 0 {
			continue
		}
		rest := raw[at+len(sep):]
		if reRawCommercialTail.FindStringSubmatch(rest) != nil {
			return true
		}
	}
	return false
}

// rawShortHeadFallback is deliberately limited to a one-token numeric head.
// It fixes X4: Foundations - Community of Planets Edition without weakening
// normal colon fallbacks such as PES 2018 or TIEBREAK.
func rawShortHeadFallback(base, raw string) string {
	colon := strings.Index(base, ":")
	if colon <= 0 {
		return ""
	}
	head := strings.TrimSpace(base[:colon])
	if len(strings.Fields(head)) != 1 || !strings.ContainsAny(head, "0123456789") {
		return ""
	}
	for _, sep := range []string{" - ", " – ", " — "} {
		at := strings.LastIndex(raw, sep)
		if at < colon {
			continue
		}
		rest := raw[at+len(sep):]
		if reRawCommercialTail.FindStringSubmatch(rest) != nil {
			return normalizeFallbackCandidate(raw[:at])
		}
	}
	return ""
}

func rawDigitalEditionFallback(raw string) string {
	for _, sep := range []string{" - ", " – ", " — "} {
		at := strings.LastIndex(raw, sep)
		if at < 0 {
			continue
		}
		rest := raw[at+len(sep):]
		m := reRawCommercialTail.FindStringSubmatch(rest)
		if m == nil || !strings.Contains(strings.ToLower(m[0]), "digital ") {
			continue
		}
		return normalizeFallbackCandidate(raw[:at])
	}
	return ""
}

var reRawPreciseEdition = regexp.MustCompile(`(?i)^\s*(?:(?:digital\s+)?limited|digital\s+deluxe|deluxe|ultimate|gold|premium|special|supporter|launch|collector['’]?s|anniversary|enhanced|definitive|director['’]s|founder['’]?s|founders|year\s+(?:one|1|two|2)|day\s+(?:one|1|two|2)|goty|soundtrack)\s+edition\b`)

func rawPreciseEditionFallback(raw string) string {
	for _, sep := range []string{" - ", " – ", " — "} {
		at := strings.LastIndex(raw, sep)
		if at < 0 {
			continue
		}
		rest := raw[at+len(sep):]
		m := reRawCommercialTail.FindStringSubmatch(rest)
		if m == nil || !reRawPreciseEdition.MatchString(m[0]) {
			continue
		}
		return normalizeFallbackCandidate(raw[:at])
	}
	return ""
}

func appendFallbackName(out []string, candidate string) []string {
	candidate = strings.Trim(candidate, " :–—-")
	if candidate == "" {
		return out
	}
	normalized := Normalize(candidate)
	for _, existing := range out {
		existingNormalized := Normalize(existing)
		if existingNormalized == normalized || strings.HasPrefix(normalized, existingNormalized+" ") {
			return out
		}
	}
	return append(out, candidate)
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
