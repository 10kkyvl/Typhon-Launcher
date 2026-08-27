package redact

import (
	"errors"
	"regexp"
	"strings"
)

const (
	Path  = "<path>"
	Token = "<token>"
	Hash  = "<hash>"
	Query = "<query>"
)

// MaxMessage and MaxStack bound anything crossing the process boundary; the
// caller truncates before the value reaches a payload, not after.
const (
	MaxMessage = 2000
	MaxStack   = 8000
)

// ErrUnsafe reports that a scrubbed value still matched a sensitive pattern.
// Callers drop the payload rather than shipping a value that failed its own
// post-condition.
var ErrUnsafe = errors.New("redact: sensitive data survived sanitization")

var (
	reMagnet = regexp.MustCompile(`(?i)magnet:\?[^\s"'<>]*`)

	// No \b anchors: two hashes written back to back leave no word boundary
	// between them, and an anchored pattern then skips the first one and
	// reports the text as clean. Over-matching a long hex run is safe here;
	// under-matching one is the leak this function exists to prevent.
	reBTIH   = regexp.MustCompile(`(?i)(?:urn:)?btih:[a-z0-9]{32,}`)
	reHex40  = regexp.MustCompile(`(?i)[a-f0-9]{40,}`)
	reBearer = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`)

	reTokenKV = regexp.MustCompile(`(?i)\b(token|access_token|refresh_token|session|sessionid|apikey|api_key|auth|authorization|password|passwd|secret|signature|sig)\b\s*[=:]\s*"?[^\s&"'<>,;]+`)

	// A URL keeps its scheme and host; everything after them can carry a feed
	// path, a release name or a query string, and none of that is diagnostic.
	reURL = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*)://([^\s/?#"'<>]*)([^\s"'<>]*)`)

	reWinUser  = regexp.MustCompile(`(?i)([a-z]:\\users\\)[^\\/:*?"<>|\r\n]+`)
	reUnixUser = regexp.MustCompile(`(?i)(/home/|/Users/)[^/\s:*?"<>|\r\n]+`)

	reWinPath  = regexp.MustCompile(`(?i)\b[a-z]:\\[^\s"'<>|\r\n]*`)
	reUNCPath  = regexp.MustCompile(`\\\\[^\s"'<>|\r\n]+`)
	reUnixPath = regexp.MustCompile(`(?:/(?:home|Users|root|var|opt|tmp|mnt|media)/)[^\s"'<>:;,)\]]*`)
)

// Text scrubs free-form text before it leaves the machine. Order matters: the
// widest and most damaging patterns run first, so a magnet URI is collapsed
// whole rather than being partially rewritten by the URL rule.
func Text(s string) string {
	if s == "" {
		return ""
	}
	s = reMagnet.ReplaceAllString(s, Magnet)
	s = reBearer.ReplaceAllString(s, "bearer "+Token)
	s = reTokenKV.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, "=:")
		if i < 0 {
			return Token
		}
		return m[:i+1] + Token
	})
	s = reBTIH.ReplaceAllString(s, "btih:"+Hash)
	s = reURL.ReplaceAllStringFunc(s, func(m string) string {
		g := reURL.FindStringSubmatch(m)
		if len(g) < 3 || g[2] == "" {
			return Hidden
		}
		host := g[2]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		return g[1] + "://" + host + "/" + Query
	})
	s = reWinUser.ReplaceAllString(s, "${1}"+Hidden)
	s = reUnixUser.ReplaceAllString(s, "${1}"+Hidden)
	s = reWinPath.ReplaceAllString(s, Path)
	s = reUNCPath.ReplaceAllString(s, Path)
	s = reUnixPath.ReplaceAllString(s, Path)
	s = reHex40.ReplaceAllString(s, Hash)
	return s
}

// Message scrubs and truncates a human-readable error message.
func Message(s string) string { return truncate(Text(s), MaxMessage) }

// Stack scrubs a Go or JavaScript stack trace. Frame structure survives; the
// absolute build and install paths that frames carry do not.
func Stack(s string) string { return truncate(Text(s), MaxStack) }

// Sanitize scrubs a value and then re-scans the result. A value that still
// matches a sensitive pattern is refused instead of shipped, so a gap in the
// rules costs a dropped report rather than a leak.
func Sanitize(s string) (string, error) {
	out := Text(s)
	if unsafeText(out) {
		return "", ErrUnsafe
	}
	return out, nil
}

func unsafeText(s string) bool {
	for _, re := range []*regexp.Regexp{reMagnet, reBTIH, reHex40, reBearer, reWinUser, reUnixUser, reWinPath, reUNCPath} {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
