package redact

import (
	"errors"
	"net/url"
	"strings"
)

const (
	Hidden = "redacted"
	Magnet = "magnet"
	File   = "torrent-file"
)

func URL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Hidden
	}
	host := u.Hostname()
	if host == "" {
		return Hidden
	}
	return u.Scheme + "://" + host
}

func Source(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "magnet:"):
		return Magnet
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return URL(raw)
	default:
		return File
	}
}

// Error strips the request URL out of *url.Error: net/http writes the whole
// URL, query string included, into the error text, and that text reaches both
// the persisted source state and the UI.
func Error(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		// Not every leaky error is a *url.Error: an fmt.Errorf that formats
		// a URL, a path or a token into its own text carries exactly the
		// same data and used to pass through untouched. Scrub the text and
		// keep the original underneath, so errors.Is and errors.As still
		// match what the caller wrapped.
		scrubbed := Text(err.Error())
		if scrubbed == err.Error() {
			return err
		}
		return &scrubbedError{msg: scrubbed, err: err}
	}
	return &url.Error{Op: ue.Op, URL: URL(ue.URL), Err: ue.Err}
}

// scrubbedError shows scrubbed text while still unwrapping to the error it
// replaced, so matching on the original sentinel keeps working.
type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }
