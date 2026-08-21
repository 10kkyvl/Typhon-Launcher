package feed

import (
	"encoding/base32"
	"encoding/hex"
	"net/url"
	"strings"
)

func MagnetInfoHash(uri string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(u.Scheme, "magnet") {
		return "", false
	}

	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", false
	}

	for _, xt := range values["xt"] {
		if hash, ok := parseXT(xt); ok {
			return hash, true
		}
	}
	return "", false
}

func parseXT(xt string) (string, bool) {
	const prefix = "urn:btih:"
	lower := strings.ToLower(xt)
	idx := strings.Index(lower, prefix)
	if idx == -1 {
		return "", false
	}
	rest := xt[idx+len(prefix):]

	switch len(rest) {
	case 40:
		if isHex(rest) {
			return strings.ToLower(rest), true
		}
	case 32:
		data, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(rest))
		if err != nil || len(data) != 20 {
			return "", false
		}
		return hex.EncodeToString(data), true
	}
	return "", false
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
