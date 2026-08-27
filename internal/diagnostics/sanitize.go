package diagnostics

import (
	"fmt"
	"unicode/utf8"

	"typhon/internal/redact"
)

const (
	maxComponentLen = 64
	maxOperationLen = 64
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

	component, err := sanitizeText(r.Component)
	if err != nil {
		return Report{}, fmt.Errorf("sanitize component: %w", err)
	}
	operation, err := sanitizeText(r.Operation)
	if err != nil {
		return Report{}, fmt.Errorf("sanitize operation: %w", err)
	}
	message, err := sanitizeText(r.Message)
	if err != nil {
		return Report{}, fmt.Errorf("sanitize message: %w", err)
	}
	stack, err := sanitizeText(r.Stack)
	if err != nil {
		return Report{}, fmt.Errorf("sanitize stack: %w", err)
	}

	r.Component = capText(component, maxComponentLen)
	r.Operation = capText(operation, maxOperationLen)
	r.Message = redact.Message(message)
	r.Stack = redact.Stack(stack)
	return r, nil
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
