package diagnostics

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"typhon/internal/redact"
)

const (
	maxComponentLen = 64
	maxOperationLen = 64

	defaultComponent = "launcher"
	defaultOperation = "unknown"
)

// sanitizeText is redact.Sanitize by default. Tests override it to force
// the fail-closed and panic-recovery branches below deterministically,
// since redact.Sanitize's own layered scrubbing leaves no known input that
// makes it fail in practice — the branches exist as a safety net for
// sensitive patterns redact does not yet know about.
var sanitizeText = redact.Sanitize

// sanitizeReport is fail-closed: every free-text field goes through
// sanitizeText (redact.Sanitize), which re-scans its own output and errors
// out if a sensitive pattern survived. Any error here means the caller must
// drop the report rather than send it. The whole pass is wrapped in
// recover so a panic inside sanitization (a future redact regex change, an
// unexpected nil) also results in a dropped report instead of a raw one
// leaving the process.
func sanitizeReport(r Report) (out Report, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			out = Report{}
			err = fmt.Errorf("diagnostics: sanitize panicked: %v", rec)
		}
	}()

	component, err := sanitizeText(dropControl(strings.ToValidUTF8(r.Component, "")))
	if err != nil {
		return Report{}, fmt.Errorf("sanitize component: %w", err)
	}
	operation, err := sanitizeText(dropControl(strings.ToValidUTF8(r.Operation, "")))
	if err != nil {
		return Report{}, fmt.Errorf("sanitize operation: %w", err)
	}
	message, err := sanitizeText(strings.ToValidUTF8(r.Message, "\uFFFD"))
	if err != nil {
		return Report{}, fmt.Errorf("sanitize message: %w", err)
	}
	stack, err := sanitizeText(strings.ToValidUTF8(r.Stack, "\uFFFD"))
	if err != nil {
		return Report{}, fmt.Errorf("sanitize stack: %w", err)
	}

	r.Component = singleLine(component, maxComponentLen, defaultComponent)
	r.Operation = singleLine(operation, maxOperationLen, defaultOperation)
	r.Message = redact.Message(stripControl(message))
	r.Stack = redact.Stack(stripControl(stack))
	r.Details = sanitizeDetails(r.Details, r.Timestamp)
	return r, nil
}

// singleLine is for fields the server checks with checkPrintable: every C0
// character and DEL is refused there, newline and tab included, and an empty
// value is refused too.
func singleLine(s string, max int, fallback string) string {
	s = capText(dropControl(s), max)
	if s == "" {
		return fallback
	}
	return s
}

// dropControl runs before redaction as well as after it: a path split by a
// control character slips past the redactor and joins up again once the
// character is dropped.
func dropControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// stripControl drops C0 control characters and DEL, keeping the three that
// carry meaning in a stack trace. A message assembled from a subprocess's
// output can carry an ANSI escape, and the server refuses a whole batch over
// one: dropping the byte here costs nothing readable and keeps a real report
// from being turned away at the border.
func stripControl(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	return r < 0x20 || r == 0x7f
}

func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
