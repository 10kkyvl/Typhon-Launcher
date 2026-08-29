package redact

import (
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
)

const (
	Path  = "<path>"
	Token = "<token>"
	Hash  = "<hash>"
	Query = "<query>"
	IP    = "<ip>"
	MAC   = "<mac>"
	Host  = "<host>"
	User  = "<user>"
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

	// Path rules match greedily to the end of the line and are trimmed back in
	// code: a path may contain spaces, and no RE2 pattern can tell
	// "C:\My Game\a.exe: denied" from a path that ends at the first space.
	// RE2 has no lookahead, so the choice is between over-matching and
	// leaking; trimPath gives back the readable tail that over-matching would
	// otherwise have swallowed.
	//
	// The \b before the drive letter keeps "https://" from reading as a drive
	// path: between "p" and "s" there is no word boundary.
	reWinPath = regexp.MustCompile(`(?i)\b[a-z]:[\\/][^"'<>|\r\n]*`)
	reUNCPath = regexp.MustCompile(`\\\\[^"'<>|\r\n]*`)

	// Absolute POSIX paths are matched by shape, not by a whitelist of leading
	// segments: a library on /data or /srv is as identifying as one on /home.
	// The leading boundary group keeps "and/or" from reading as a path, and
	// requiring a non-slash character after the slash keeps an already
	// substituted "host/<query>" from matching again.
	reUnixPath = regexp.MustCompile(`(^|[\s"'(\[=,])(/[^\s"'<>|\r\n/]+(?:/[^\s"'<>|\r\n]*)?)`)

	// A trailing "file.go:305 +0x1a4" is put back after the path is replaced:
	// line numbers carry no identity and are what makes a stack actionable.
	reLocSuffix = regexp.MustCompile(`(?i):\d+(?::\d+)?(?: \+0x[0-9a-f]+)?$`)

	reMAC  = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`)
	reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

	// Two forms, because "::" is what makes the compressed one unambiguous.
	// The full form demands all eight groups and the compressed one demands a
	// literal "::", so neither can match a stack frame's ":305:12".
	reIPv6Full = regexp.MustCompile(`(?i)\b[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){7}\b`)
	reIPv6Comp = regexp.MustCompile(`(?i)(?:\b[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?::(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?`)

	// Windows names a machine DESKTOP-XXXXXXX unless the owner renames it, and
	// owners who rename it tend to use their own name. The literal hostname is
	// removed separately by SetLocal; this is the net for the machines whose
	// name never reached the process.
	reGenericHost = regexp.MustCompile(`(?i)\b(?:desktop|laptop)-[a-z0-9]{4,}\b`)
)

// localIdentity holds the literal machine and account names of the running
// install. No pattern can recognise a username like "egor" outside a path, so
// the values are matched literally instead of guessed.
type localIdentity struct {
	host *regexp.Regexp
	user *regexp.Regexp
}

var local atomic.Pointer[localIdentity]

// genericNames are account names too common to replace: substituting every
// "user" or "admin" in a message would destroy the text without hiding anyone.
var genericNames = map[string]bool{
	"admin": true, "administrator": true, "user": true, "users": true,
	"guest": true, "public": true, "default": true, "owner": true, "home": true,
}

// SetLocal registers this machine's hostname and account name so they are
// removed from free text wherever they appear, not only inside a path. Empty,
// very short and generic values are ignored: they would match unrelated words.
// Values that are all ignored leave the generic patterns above in force.
func SetLocal(hostname, username string) {
	id := &localIdentity{
		host: literalPattern(hostname),
		user: literalPattern(username),
	}
	local.Store(id)
}

func literalPattern(v string) *regexp.Regexp {
	v = strings.TrimSpace(v)
	if len([]rune(v)) < 3 || genericNames[strings.ToLower(v)] {
		return nil
	}
	return regexp.MustCompile(`(?i)` + regexp.QuoteMeta(v))
}

// Text scrubs free-form text before it leaves the machine. Order matters:
// URLs collapse before the token rules run, because a token rule that fires
// first injects "<token>" into the URL and the URL pattern stops at '<',
// leaving the rest of the query string outside the match and on the wire.
func Text(s string) string {
	if s == "" {
		return ""
	}
	s = reMagnet.ReplaceAllString(s, Magnet)
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
	s = reBearer.ReplaceAllString(s, "bearer "+Token)
	s = reTokenKV.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, "=:")
		if i < 0 {
			return Token
		}
		return m[:i+1] + Token
	})
	s = reBTIH.ReplaceAllString(s, "btih:"+Hash)

	s = reWinPath.ReplaceAllStringFunc(s, func(m string) string { return trimPath(m, 2) })
	s = reUNCPath.ReplaceAllStringFunc(s, func(m string) string { return trimPath(m, 2) })
	s = reUnixPath.ReplaceAllStringFunc(s, func(m string) string {
		g := reUnixPath.FindStringSubmatch(m)
		if len(g) < 3 {
			return Path
		}
		return g[1] + trimPath(g[2], 1)
	})

	s = reMAC.ReplaceAllString(s, MAC)
	s = reIPv6Full.ReplaceAllString(s, IP)
	s = reIPv6Comp.ReplaceAllString(s, IP)
	s = reIPv4.ReplaceAllString(s, IP)

	s = reGenericHost.ReplaceAllString(s, Host)
	if id := local.Load(); id != nil {
		if id.host != nil {
			s = id.host.ReplaceAllString(s, Host)
		}
		if id.user != nil {
			s = id.user.ReplaceAllString(s, User)
		}
	}

	s = reHex40.ReplaceAllString(s, Hash)
	return s
}

// trimPath replaces a greedily matched path with the placeholder and gives
// back the tail the greedy match swallowed: the "op path: message" separator
// Go errors use, and a trailing source location. skip is the offset past the
// prefix that carries its own colon, so the "D:" of a drive and the leading
// slashes of a UNC share are not mistaken for that separator.
func trimPath(m string, skip int) string {
	if len(m) < skip {
		return Path
	}
	body, tail := m, ""
	if i := strings.Index(body[skip:], ": "); i >= 0 {
		cut := skip + i
		body, tail = body[:cut], body[cut:]
	}
	if loc := reLocSuffix.FindString(body); loc != "" {
		tail = loc + tail
	}
	return Path + tail
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

// unsafeText lists every pattern Text can substitute. A rule added to Text but
// not added here would weaken the fail-closed contract silently.
func unsafeText(s string) bool {
	for _, re := range []*regexp.Regexp{
		reMagnet, reBTIH, reHex40, reBearer,
		reWinPath, reUNCPath, reUnixPath,
		reMAC, reIPv6Full, reIPv6Comp, reIPv4, reGenericHost,
	} {
		if re.MatchString(s) {
			return true
		}
	}
	if id := local.Load(); id != nil {
		if id.host != nil && id.host.MatchString(s) {
			return true
		}
		if id.user != nil && id.user.MatchString(s) {
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
