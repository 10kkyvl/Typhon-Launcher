package install

import (
	"strings"
	"unicode/utf8"

	"typhon/internal/redact"
)

// A report message is cut at redact.MaxMessage; the tail has to leave room for
// the exit error in front of it.
const installerReportTailLimit = 1200

type loggedFailure struct {
	err  error
	tail string
}

func (e *loggedFailure) Error() string { return e.err.Error() + "\ninstaller log tail:\n" + e.tail }
func (e *loggedFailure) Unwrap() error { return e.err }

// withLogTail is for the log record that becomes a diagnostics report: the
// report keeps only the message and the error of a record, not its other
// attributes, so the installer log has to travel inside the error.
func withLogTail(err error, tail string) error {
	tail = reportLogTail(tail)
	if tail == "" {
		return err
	}
	return &loggedFailure{err: err, tail: tail}
}

func reportLogTail(tail string) string {
	if tail == "" {
		return ""
	}
	clean, err := redact.Sanitize(tail)
	if err != nil {
		return "installer log withheld: " + err.Error()
	}
	const cut = "…\n"
	if len(clean) <= installerReportTailLimit {
		return clean
	}
	clean = clean[len(clean)-(installerReportTailLimit-len(cut)):]
	for len(clean) > 0 && !utf8.RuneStart(clean[0]) {
		clean = clean[1:]
	}
	if i := strings.IndexByte(clean, '\n'); i >= 0 && i+1 < len(clean) {
		clean = clean[i+1:]
	}
	return cut + clean
}
