package support

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/account"
)

func invalidField(code, field string) error {
	return &account.Error{Code: code, Field: field, Status: http.StatusUnprocessableEntity}
}

func normalizeSubject(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(v)
	if !utf8.ValidString(v) || n < 1 || n > MaxSubjectRunes {
		return "", invalidField(CodeTicketInvalidSubject, "subject")
	}
	for _, r := range v {
		if invisibleInLine(r) {
			return "", invalidField(CodeTicketInvalidSubject, "subject")
		}
	}
	return v, nil
}

func normalizeBody(raw string) (string, error) {
	v := strings.ReplaceAll(raw, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if !utf8.ValidString(v) || n < 1 || n > MaxBodyRunes {
		return "", invalidField(CodeTicketInvalidBody, "body")
	}
	for _, r := range v {
		if invisibleInBody(r) {
			return "", invalidField(CodeTicketInvalidBody, "body")
		}
	}
	return v, nil
}

func invisibleInLine(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf)
}

func invisibleInBody(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case unicode.IsControl(r):
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0x2028, r == 0x2029, r == 0xFEFF:
		return true
	}
	return false
}
