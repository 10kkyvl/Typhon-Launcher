package titles

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// Words a feed puts after a dash as packaging labels. A tail made only of
	// these is not part of the title: "Game - Release HV" is just "Game",
	// while "Rebirth - Repentance" keeps its second half.
	reJunkDashToken = regexp.MustCompile(`(?i)^(?:release|hv|non-hv|nonhv|x86|x64|win32|win64|steam|gog|complete|ptbr|denuvoless|crackfix|testks|hf\d*|rc\d*|(?:patch|hotfix|build|update)\d*|v\d[\w.]*|b\d[\w.]*|\d+(?:\.\d+)+\w*|\d{5,}\w*)$`)

	reJunkStrongToken = regexp.MustCompile(`(?i)^(?:v\d[\w.]*|b\d[\w.]*|r\d+|rc\d*|(?:build|patch|update|hotfix|hf|release|ver|version|latest)\d*|\d+(?:\.\d+)+\w*|\d{5,}\w*)$`)
	reJunkWeakToken   = regexp.MustCompile(`(?i)^\d+[a-z]?$`)
	reJunkStoreToken  = regexp.MustCompile(`(?i)^(?:gog|steam|x64|x86)$`)

	reJunkEpisodeTail = regexp.MustCompile(`(?i)[\s:–—,-]*\(?\b(?:complete\s+season(?:\s+\d+)?|(?:episodes?|chapters?)\s+\d+(?:(?:\s*[-–&,]\s*|\s+and\s+)\d+)*)\)?\s*$`)

	reLatinLetter = regexp.MustCompile(`\p{Latin}`)
)

// junkFallbackNames offers shorter spellings for titles that a feed buried
// under version labels, packaging words, episode ranges or a second title.
// They are only appended after every spelling the exact lookup already had,
// so a game that really carries such words in its name is found first.
func junkFallbackNames(cleaned, base string, existing []string) []string {
	acc := slices.Clone(existing)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		p := Parse(candidate)
		if p.Base == "" || strings.IndexFunc(p.Base, unicode.IsLetter) < 0 {
			return
		}
		names := []string{normalizeFallbackCandidate(candidate)}
		if p.Edition != "" && !identityMatchEdition(p.Edition) {
			names = append(names, p.Base)
		}
		for _, name := range names {
			if key := Normalize(name); key != "" && !slices.ContainsFunc(acc, func(have string) bool { return Normalize(have) == key }) {
				acc = append(acc, strings.Trim(name, " :–—-"))
			}
		}
	}

	// Source markers after the title must not hide the version or dash
	// labels in front of them.
	seed, _ := Active().extractMarkers(cleaned)
	seed = strings.TrimSpace(seed)
	if strings.Contains(seed, " / ") {
		// The bilingual and GTA rules in matchNames already resolved this
		// slash, or left both titles in base: either way the parts are
		// cleaned on their own and the whole line is not cut again.
		for _, part := range splitSlashTitles(base) {
			add(stripJunkTail(cutAtPlus(part)))
		}
		return acc[len(existing):]
	}
	head := cutAtPlus(seed)
	if head != seed {
		add(head)
	}
	stripped := stripJunkTail(head)
	if stripped != head {
		add(stripped)
	}
	if cut := cutCyrillicTail(stripped); cut != stripped {
		add(cut)
	}
	return acc[len(existing):]
}

// cutAtPlus returns the title before the first " +" outside brackets. The plus
// must follow a space: "PAC-MAN MUSEUM+" is a title, "Game + Bonus" is not.
func cutAtPlus(s string) string {
	depth := 0
	for i := range len(s) {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth = max(0, depth-1)
		case '+':
			if depth > 0 || i == 0 || s[i-1] != ' ' {
				continue
			}
			head := strings.TrimSpace(s[:i])
			if utf8.RuneCountInString(head) < 3 || strings.TrimSpace(s[i+1:]) == "" {
				return s
			}
			return head
		}
	}
	return s
}

func stripJunkTail(s string) string {
	s = strings.TrimSpace(s)
	for {
		next, ok := trimVersionBracket(s)
		if !ok {
			next, ok = trimDashJunk(s)
		}
		if !ok {
			next, ok = trimEpisodeTail(s)
		}
		if !ok {
			next, ok = trimVersionTail(s)
		}
		if !ok {
			return s
		}
		s = next
	}
}

func hasTitleLetters(s string) bool {
	return strings.IndexFunc(s, unicode.IsLetter) >= 0
}

func trimVersionBracket(s string) (string, bool) {
	if s == "" {
		return s, false
	}
	var opener string
	switch s[len(s)-1] {
	case ')':
		opener = "("
	case ']':
		opener = "["
	default:
		return s, false
	}
	at := strings.LastIndex(s, opener)
	if at <= 0 || !versionBracket(s[at+1:len(s)-1]) {
		return s, false
	}
	head := strings.TrimSpace(s[:at])
	if !hasTitleLetters(head) {
		return s, false
	}
	return head, true
}

// versionBracket accepts a bracket whose every token is a version, build or
// store label, such as "(1.0-v4)", "(b192-v2)", "(RC2)", "(release-53)".
// A bare number or "(3D)" stays: it can belong to the title.
func versionBracket(inner string) bool {
	tokens := strings.FieldsFunc(inner, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("-_,+/", r)
	})
	if len(tokens) == 0 {
		return false
	}
	anchored := false
	for _, token := range tokens {
		switch {
		case reJunkStrongToken.MatchString(token), reJunkStoreToken.MatchString(token):
			anchored = true
		case reJunkWeakToken.MatchString(token):
		default:
			return false
		}
	}
	return anchored
}

func trimDashJunk(s string) (string, bool) {
	for _, sep := range []string{" - ", " – ", " — "} {
		at := strings.LastIndex(s, sep)
		if at <= 0 {
			continue
		}
		tail := s[at+len(sep):]
		tokens := strings.FieldsFunc(tail, func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune(",/+&", r)
		})
		if len(tokens) == 0 {
			continue
		}
		junk := true
		for _, token := range tokens {
			if !reJunkDashToken.MatchString(token) {
				junk = false
				break
			}
		}
		head := strings.TrimSpace(s[:at])
		if junk && hasTitleLetters(head) {
			return head, true
		}
	}
	return s, false
}

func trimEpisodeTail(s string) (string, bool) {
	loc := reJunkEpisodeTail.FindStringIndex(s)
	if loc == nil || loc[0] == 0 {
		return s, false
	}
	head := strings.TrimSpace(s[:loc[0]])
	if !hasTitleLetters(head) {
		return s, false
	}
	return head, true
}

// trimVersionTail drops a release version and everything the feed wrote after
// it, so the dash and bracket rules see the real end of the title.
func trimVersionTail(s string) (string, bool) {
	loc := versionLocation(s)
	if loc == nil || loc[0] <= 0 {
		return s, false
	}
	head := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s[:loc[0]]), "([{ -–—,:"))
	if !hasTitleLetters(head) {
		return s, false
	}
	return head, true
}

// splitSlashTitles splits "A / B" into its titles. Only a slash with spaces on
// both sides separates titles: "Campaign/Zombies" is one name.
func splitSlashTitles(s string) []string {
	if !strings.Contains(s, " / ") {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, " / ") {
		part = strings.TrimSpace(part)
		if utf8.RuneCountInString(part) >= 3 && hasTitleLetters(part) {
			out = append(out, part)
		}
	}
	return out
}

// cutCyrillicTail removes a Russian subtitle glued to a Latin title without
// any separator: "The Movies Фабрика грёз" is "The Movies".
func cutCyrillicTail(s string) string {
	words := strings.Fields(Parse(s).Base)
	end := len(words)
	for end > 0 && hasCyrillic(words[end-1]) && !reLatinLetter.MatchString(words[end-1]) {
		end--
	}
	if end == len(words) || end == 0 {
		return s
	}
	head := strings.Join(words[:end], " ")
	if hasCyrillic(head) || !reLatinLetter.MatchString(head) {
		return s
	}
	return head
}
