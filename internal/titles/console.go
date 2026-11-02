package titles

import (
	"regexp"
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
	consoleName = `(?:(ps[ ._-]?(?:1|one)|psx|playstation[ ._-]?(?:1|one))|(ps[ ._-]?2|playstation[ ._-]?2)|(psp|playstation[ ._-]?portable))`
	consoleMeta = `iso|cso|chd|pbp|ntsc(?:-[uj])?|pal|rus|eng|jpn|multi\d*`
)

var (
	reConsoleName = regexp.MustCompile(`(?i)` + consoleName)
	// One comma-separated part of a bracket: «PS2», «PS2 NTSC», but not «PS2 Remake».
	reConsoleBracketPart = regexp.MustCompile(`(?i)^\s*` + consoleName + `((?:[\s._]+(?:` + consoleMeta + `))*)\s*$`)
	// What may follow a bare token: release data, never words of a title.
	reConsoleMetaTail = regexp.MustCompile(`(?i)^(?:(?:` + consoleMeta + `|v\.?\d\S*|(?:19|20)\d\d)(?:[\s._]+|$))+$`)
	reConsolePrep     = regexp.MustCompile(`(?i)[\s._](?:for|для|на)$`)
	// Disc image and region labels only appear beside a console token; the same
	// words stay untouched in a title that has none.
	reConsoleRegion = regexp.MustCompile(`(?i)[\[({]\s*(?:ntsc(?:-[uj])?|pal|usa|europe|japan)\s*[\])}]`)
	reConsoleImage  = regexp.MustCompile(`(?i)[\s._-]+(?:iso|cso|chd|pbp|ntsc(?:-[uj])?|pal)[\s._-]*$`)
)

func consoleFromGroups(loc []int) string {
	for i, id := range Consoles() {
		if loc[2+2*i] >= 0 {
			return id
		}
	}
	return ""
}

// extractConsole finds a console named on its own and cuts it out of the title,
// so that it never reaches the server. Nothing is touched without such a token.
// A release naming two consoles takes the first one; every token is cut.
func extractConsole(s string) (string, string) {
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
	// «PlayStation 2» alone is a name, not a label on a release.
	if strings.TrimSpace(Normalize(cut)) == "" {
		return s, ""
	}
	return cut, id
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
			loc := reConsoleBracketPart.FindStringSubmatchIndex(part)
			if loc == nil {
				kept = append(kept, part)
				continue
			}
			cut = true
			if id == "" {
				id = consoleFromGroups(loc)
			}
			if meta := strings.TrimSpace(part[loc[8]:loc[9]]); meta != "" {
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

// cutConsoleWord takes a token outside brackets. It counts at the end of the
// title (only release data may follow), between two separators, or first and
// followed by a separator. In the middle of words it belongs to the title.
func cutConsoleWord(s string) (string, string) {
	for _, loc := range reConsoleName.FindAllStringSubmatchIndex(s, -1) {
		before, after := s[:loc[0]], s[loc[1]:]
		if !consoleBoundary(before, after) {
			continue
		}
		head := strings.TrimRight(before, " ._")
		tail := strings.TrimLeft(after, " ._")
		sepAfter := consoleSeparatorStart(tail)
		switch {
		case head == "":
			if !sepAfter {
				continue
			}
		case consoleSeparatorEnd(head) && sepAfter:
		case tail == "" || strings.ContainsRune("[({|", rune(tail[0])) || reConsoleMetaTail.MatchString(tail):
		default:
			continue
		}
		if prep := reConsolePrep.FindStringIndex(head); prep != nil {
			before = head[:prep[0]]
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

func consoleSeparatorEnd(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune("-|/,:–—", r)
}

func consoleSeparatorStart(s string) bool {
	if s == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(s)
	if !strings.ContainsRune("-|/,:–—", r) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(s[size:])
	return next == ' ' || next == utf8.RuneError
}
