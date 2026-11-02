package titles

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The ids repeat the table in internal/emulators: that package depends on
// catalog, and catalog on this one. A test in internal/sources keeps both equal.
const (
	ConsolePS1 = "ps1"
	ConsolePS2 = "ps2"
	ConsolePSP = "psp"
)

func Consoles() []string {
	return []string{ConsolePS1, ConsolePS2, ConsolePSP}
}

const (
	consoleName = `(?:sony[ ._-]+)?(?:(ps[ ._-]?(?:1|one)|psx|playstation[ ._-]?(?:1|one))|(ps[ ._-]?2|playstation[ ._-]?2)|(psp|playstation[ ._-]?portable)|(playstation))`
	consoleMeta = `iso|cso|chd|pbp|ntsc(?:-[uj])?|pal|rus|eng|jpn|multi\d*`
	pcName      = `(?:pc|windows|win)`

	consoleSeparators = "-|/,:–—"
)

var (
	reConsoleName = regexp.MustCompile(`(?i)` + consoleName)
	// One part of a bracket: «PS2», «PS2 NTSC», but not «PS2 Remake».
	reConsoleBracketPart = regexp.MustCompile(`(?i)^\s*` + consoleName + `((?:[\s._]+(?:` + consoleMeta + `))*)\s*$`)
	// What may follow a bare token: release data, never words of a title.
	reConsoleMetaTail = regexp.MustCompile(`(?i)^(?:(?:` + consoleMeta + `|v\.?\d\S*|(?:19|20)\d\d)(?:[\s._]+|$))+$`)
	rePCBracketPart   = regexp.MustCompile(`(?i)^\s*` + pcName + `(?:[ ._]?(?:32|64|xp|7|8|10|11))?\s*$`)
	rePCBeforeConsole = regexp.MustCompile(`(?i)(?:^|[\s._])` + pcName + `\s*[` + consoleSeparators + `]*\s*$`)
	rePCAfterConsole  = regexp.MustCompile(`(?i)^[` + consoleSeparators + `]\s*` + pcName + `\b`)
	// Disc image and region labels only appear beside a console token; the same
	// words stay untouched in a title that has none.
	reConsoleRegion = regexp.MustCompile(`(?i)[\[({]\s*(?:ntsc(?:-[uj])?|pal|usa|europe|japan)\s*[\])}]`)
	reConsoleImage  = regexp.MustCompile(`(?i)[\s._-]+(?:iso|cso|chd|pbp|ntsc(?:-[uj])?|pal)[\s._-]*$`)
)

func consoleFromGroups(loc []int) string {
	for i, id := range []string{ConsolePS1, ConsolePS2, ConsolePSP, ConsolePS1} {
		if loc[2+2*i] >= 0 {
			return id
		}
	}
	return ""
}

// extractConsole finds a console named on its own and cuts it out of the title,
// so that it never reaches the server. Nothing is touched without such a token,
// and a bracket that also names the PC keeps the release a PC one. A release
// naming two consoles takes the first one; every token is cut.
func extractConsole(s string) (string, string) {
	if hasPCBracket(s) {
		return s, ""
	}
	cut, id := cutConsoleBrackets(s)
	for {
		next, found := cutConsoleWord(cut)
		if found == "" {
			break
		}
		cut = next
		if id == "" {
			id = found
		}
	}
	if id == "" {
		return s, ""
	}
	cut = reConsoleRegion.ReplaceAllString(cut, " ")
	for {
		next := reConsoleImage.ReplaceAllString(cut, "")
		if next == cut {
			break
		}
		cut = next
	}
	cut = strings.Trim(cut, " "+consoleSeparators)
	// «PlayStation 2» alone is a name, not a label on a release.
	if Normalize(cut) == "" {
		return s, ""
	}
	return cut, id
}

func hasPCBracket(s string) bool {
	for _, bracket := range reBracket.FindAllString(s, -1) {
		parts := strings.FieldsFunc(bracket[1:len(bracket)-1], func(r rune) bool {
			return strings.ContainsRune(",;/|+-", r)
		})
		if slices.ContainsFunc(parts, rePCBracketPart.MatchString) {
			return true
		}
	}
	return false
}

// consoleBracketPart reads one part of a bracket. «PSX-PSP» is two consoles
// joined by a dash, while «PS-2» is one, so the whole part is tried first.
func consoleBracketPart(part string) (id, meta string, ok bool) {
	if loc := reConsoleBracketPart.FindStringSubmatchIndex(part); loc != nil {
		return consoleFromGroups(loc), strings.TrimSpace(part[loc[10]:loc[11]]), true
	}
	if !strings.Contains(part, "-") {
		return "", "", false
	}
	for _, sub := range strings.Split(part, "-") {
		loc := reConsoleBracketPart.FindStringSubmatchIndex(sub)
		if loc == nil {
			return "", "", false
		}
		if id == "" {
			id = consoleFromGroups(loc)
		}
	}
	return id, "", true
}

func cutConsoleBrackets(s string) (string, string) {
	id := ""
	out := reBracket.ReplaceAllStringFunc(s, func(bracket string) string {
		parts := strings.FieldsFunc(bracket[1:len(bracket)-1], func(r rune) bool {
			return strings.ContainsRune(",;/|+", r)
		})
		kept := make([]string, 0, len(parts))
		cut := false
		for _, part := range parts {
			found, meta, ok := consoleBracketPart(part)
			if !ok {
				kept = append(kept, part)
				continue
			}
			cut = true
			if id == "" {
				id = found
			}
			if meta != "" {
				kept = append(kept, meta)
			}
		}
		if !cut {
			return bracket
		}
		if len(kept) == 0 {
			return " "
		}
		return bracket[:1] + strings.Join(kept, ",") + bracket[len(bracket)-1:]
	})
	return out, id
}

// cutConsoleWord takes a token outside brackets. It counts behind a separator,
// when only release data, a bracket or another separator follows, or first and
// followed by a separator. A word with nothing but a space before it belongs to
// the title: «Bloodborne PSX» is a game of that name. The bare brand name counts
// only in a bracket.
func cutConsoleWord(s string) (string, string) {
	for _, loc := range reConsoleName.FindAllStringSubmatchIndex(s, -1) {
		before, after := s[:loc[0]], s[loc[1]:]
		if !consoleBoundary(before, after) || loc[8] == loc[0] {
			continue
		}
		head := strings.TrimRight(before, " ._")
		tail := strings.TrimLeft(after, " ._")
		if rePCBeforeConsole.MatchString(head) || rePCAfterConsole.MatchString(tail) {
			continue
		}
		sepAfter := consoleSeparatorStart(tail)
		switch {
		case head == "":
			if !sepAfter {
				continue
			}
		case !consoleSeparatorEnd(head):
			continue
		case tail == "" || sepAfter || strings.ContainsRune("[({|", rune(tail[0])) || reConsoleMetaTail.MatchString(tail):
			before = strings.TrimRight(head, " "+consoleSeparators)
		default:
			continue
		}
		return before + " " + after, consoleFromGroups(loc)
	}
	return s, ""
}

func consoleBoundary(before, after string) bool {
	if r, _ := utf8.DecodeLastRuneInString(before); unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	r, size := utf8.DecodeRuneInString(after)
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	// «PS2.5» is a number, not a console.
	if r == '.' {
		next, _ := utf8.DecodeRuneInString(after[size:])
		return !unicode.IsDigit(next)
	}
	return true
}

// A dash glued to the word is a hyphen: «Half-PSX» is a title.
func consoleSeparatorEnd(s string) bool {
	return strings.HasSuffix(s, " -") || strings.HasSuffix(s, "|") || strings.HasSuffix(s, "/") ||
		strings.HasSuffix(s, "–") || strings.HasSuffix(s, "—")
}

func consoleSeparatorStart(s string) bool {
	if s == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(s)
	if !strings.ContainsRune(consoleSeparators, r) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(s[size:])
	return next == ' ' || next == utf8.RuneError
}
