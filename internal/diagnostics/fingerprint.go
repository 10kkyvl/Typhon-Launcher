package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const fingerprintFrames = 3

// Fingerprint groups reports for client-side rate limiting and dedup, and
// must match what the backend computes independently from the same
// reportPayload fields. Normalization, exactly:
//
//	sha256(error_code + "\n" + component + "\n" + frame1 + "|" + frame2 + "|" + frame3)
//
// where error_code and component are lower-cased and trimmed of surrounding
// whitespace, and frame1..3 are the identities of the first three stack
// frames found in stack, in encounter order, joined with "|". A frame
// identity is its function/symbol name with any file path, line number,
// column, or memory address stripped; a frame whose location could not be
// resolved to a symbol (a minified or anonymous JS frame) becomes the
// literal "<anonymous>" rather than being dropped, so its position in the
// sequence still counts. Lines that carry only a file path and line number
// (a Go stack's continuation line) are not frames and are skipped entirely,
// and so is a leading "goroutine N [running]:" header or an "ErrorType:
// message" header line (a line with neither an "at " prefix nor a '(').
// Fewer than three resolvable frames means fewer segments before the last
// "|" — the missing slots are omitted, not padded with empty strings.
func Fingerprint(errorCode, component, stack string) string {
	ec := strings.ToLower(strings.TrimSpace(errorCode))
	comp := strings.ToLower(strings.TrimSpace(component))
	frames := normalizeFrames(stack, fingerprintFrames)
	sum := sha256.Sum256([]byte(ec + "\n" + comp + "\n" + strings.Join(frames, "|")))
	return hex.EncodeToString(sum[:])
}

func normalizeFrames(stack string, n int) []string {
	frames := make([]string, 0, n)
	for _, line := range strings.Split(stack, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "goroutine ") {
			continue
		}
		ident := frameIdentity(line)
		if ident == "" {
			continue
		}
		frames = append(frames, ident)
		if len(frames) == n {
			break
		}
	}
	return frames
}

// frameIdentity extracts the callable name from one stack trace line. It
// handles both Go's "pkg.(*Type).Method(args)" / "\t/path/file.go:10 +0x25"
// pairs (runtime/debug.Stack) and JS's "at Foo (file:line:col)" /
// "at file:line:col" forms (browser stacks reported through
// ReportClientError). The last '(' on the line is treated as the start of
// the argument list, not the first, because Go method frames embed a
// receiver type in parens before the call: "pkg.(*Type).Method(0x1234)".
func frameIdentity(line string) string {
	if looksLikePathLine(line) {
		return ""
	}
	isFrame := strings.HasPrefix(line, "at ") || strings.ContainsRune(line, '(')
	if !isFrame {
		// A leading "ErrorType: message" line (JS Error.stack, a panic
		// value) is not a frame; only genuine call frames reach here.
		return ""
	}
	line = strings.TrimPrefix(line, "at ")
	if i := strings.LastIndexByte(line, '('); i >= 0 {
		fn := strings.TrimSpace(line[:i])
		if fn == "" {
			return "<anonymous>"
		}
		return fn
	}
	if looksLikePathLine(line) {
		return "<anonymous>"
	}
	return strings.TrimSpace(line)
}

func looksLikePathLine(s string) bool {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return false
	case strings.HasPrefix(s, "/"):
		return true
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"):
		return true
	case len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/'):
		return true
	default:
		return false
	}
}
