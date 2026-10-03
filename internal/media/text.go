package media

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxText    = 200
	maxAppName = 60
)

var packageSuffix = regexp.MustCompile(`_[a-z0-9]{13}$`)

func clean(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	n := 0
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			r = ' '
		case unicode.IsControl(r) || isBidiControl(r):
			continue
		}
		if n == limit {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

func isBidiControl(r rune) bool {
	switch {
	case r == 0x200E || r == 0x200F:
		return true
	case r >= 0x202A && r <= 0x202E:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

func shortAppName(id string) string {
	s := clean(id, maxText)
	pkg := false
	if i := strings.LastIndex(s, "!"); i >= 0 {
		head, app := s[:i], s[i+1:]
		pkg = true
		if app == "" || strings.EqualFold(app, "app") {
			s = packageSuffix.ReplaceAllString(head, "")
		} else {
			s = app
		}
	} else if packageSuffix.MatchString(s) {
		s = packageSuffix.ReplaceAllString(s, "")
		pkg = true
	}
	if len(s) > 4 && strings.EqualFold(s[len(s)-4:], ".exe") {
		s = s[:len(s)-4]
	}
	if pkg {
		if i := strings.LastIndex(s, "."); i >= 0 && i < len(s)-1 {
			s = s[i+1:]
		}
	}
	s = strings.TrimSpace(s)
	if r, size := utf8.DecodeRuneInString(s); size > 0 && unicode.IsLower(r) {
		s = string(unicode.ToUpper(r)) + s[size:]
	}
	return clean(s, maxAppName)
}
