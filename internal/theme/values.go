package theme

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	numberRe   = regexp.MustCompile(`^-?(?:\d+\.?\d*|\.\d+)$`)
	percentRe  = regexp.MustCompile(`^-?(?:\d+\.?\d*|\.\d+)%$`)
	lengthRe   = regexp.MustCompile(`^-?(?:\d+\.?\d*|\.\d+)(?:px|rem|em|%|vw|vh)$`)
	timeRe     = regexp.MustCompile(`^(?:\d+\.?\d*|\.\d+)(?:ms|s)$`)
	hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	angleRe    = regexp.MustCompile(`^-?(?:\d+\.?\d*|\.\d+)(?:deg)?$`)
	familyRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 -]*$`)
)

var (
	easeKeywords   = map[string]bool{"linear": true, "ease": true, "ease-in": true, "ease-out": true, "ease-in-out": true}
	genericFamily  = map[string]bool{"serif": true, "sans-serif": true, "monospace": true, "cursive": true, "fantasy": true, "system-ui": true, "ui-sans-serif": true, "ui-serif": true, "ui-monospace": true, "ui-rounded": true}
	colorMixSpaces = map[string]bool{"srgb": true, "oklab": true, "hsl": true}
)

func validTokenKind(kind Kind, value string) bool {
	v := strings.TrimSpace(value)
	switch kind {
	case KindColor:
		return isColor(v)
	case KindLength:
		return isLength(v)
	case KindNumber:
		return numberRe.MatchString(v)
	case KindTime:
		return timeRe.MatchString(v)
	case KindEase:
		return isEase(v)
	case KindShadow:
		return isShadow(v)
	case KindFont:
		return isFont(v)
	}
	return false
}

func funcArgs(v, name string) (string, bool) {
	prefix := name + "("
	if len(v) <= len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) || !strings.HasSuffix(v, ")") {
		return "", false
	}
	inner := v[len(prefix) : len(v)-1]
	depth := 0
	for _, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return "", false
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	return inner, true
}

func splitTop(s string, sep rune) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + len(string(r))
			}
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

func fieldsTop(s string) []string {
	var parts []string
	depth, start := 0, -1
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
		if r == ' ' && depth == 0 {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

func isColor(v string) bool {
	if hexColorRe.MatchString(v) || strings.EqualFold(v, "transparent") {
		return true
	}
	for _, name := range []string{"rgb", "rgba"} {
		if inner, ok := funcArgs(v, name); ok {
			return colorChannels(inner, false)
		}
	}
	for _, name := range []string{"hsl", "hsla"} {
		if inner, ok := funcArgs(v, name); ok {
			return colorChannels(inner, true)
		}
	}
	if inner, ok := funcArgs(v, "color-mix"); ok {
		return isColorMix(inner)
	}
	return false
}

func colorChannels(inner string, hue bool) bool {
	parts := strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == '/' || r == ' ' })
	if len(parts) != 3 && len(parts) != 4 {
		return false
	}
	for i, p := range parts {
		switch {
		case hue && i == 0:
			if !angleRe.MatchString(p) {
				return false
			}
		case !numberRe.MatchString(p) && !percentRe.MatchString(p):
			return false
		}
	}
	return true
}

func isColorMix(inner string) bool {
	parts := splitTop(inner, ',')
	if len(parts) != 3 {
		return false
	}
	head := strings.Fields(parts[0])
	if len(head) != 2 || head[0] != "in" || !colorMixSpaces[head[1]] {
		return false
	}
	return isMixStop(parts[1]) && isMixStop(parts[2])
}

func isMixStop(s string) bool {
	fields := fieldsTop(s)
	switch len(fields) {
	case 1:
		return isColor(fields[0])
	case 2:
		if percentRe.MatchString(fields[1]) {
			return isColor(fields[0])
		}
		if percentRe.MatchString(fields[0]) {
			return isColor(fields[1])
		}
	}
	return false
}

func isLength(v string) bool {
	if v == "0" || lengthRe.MatchString(v) {
		return true
	}
	if inner, ok := funcArgs(v, "calc"); ok {
		return isCalc(inner)
	}
	for _, name := range []string{"min", "max", "clamp"} {
		inner, ok := funcArgs(v, name)
		if !ok {
			continue
		}
		args := splitTop(inner, ',')
		if name == "clamp" && len(args) != 3 || len(args) < 2 {
			return false
		}
		for _, a := range args {
			if !isLength(a) {
				return false
			}
		}
		return true
	}
	return false
}

func isCalc(expr string) bool {
	padded := strings.NewReplacer("(", " ( ", ")", " ) ").Replace(expr)
	depth := 0
	wantOperand := true
	for _, tok := range strings.Fields(padded) {
		switch {
		case tok == "(":
			if !wantOperand {
				return false
			}
			depth++
		case tok == ")":
			depth--
			if depth < 0 || wantOperand {
				return false
			}
		case tok == "+" || tok == "-" || tok == "*" || tok == "/":
			if wantOperand {
				return false
			}
			wantOperand = true
		case lengthRe.MatchString(tok) || numberRe.MatchString(tok):
			if !wantOperand {
				return false
			}
			wantOperand = false
		default:
			return false
		}
	}
	return depth == 0 && !wantOperand
}

func isEase(v string) bool {
	if easeKeywords[v] {
		return true
	}
	inner, ok := funcArgs(v, "cubic-bezier")
	if !ok {
		return false
	}
	args := splitTop(inner, ',')
	if len(args) != 4 {
		return false
	}
	for i, a := range args {
		n, err := strconv.ParseFloat(a, 64)
		if err != nil || !numberRe.MatchString(a) {
			return false
		}
		if i%2 == 0 && (n < 0 || n > 1) {
			return false
		}
	}
	return true
}

func isShadow(v string) bool {
	if v == "none" {
		return true
	}
	for _, one := range splitTop(v, ',') {
		if !isOneShadow(one) {
			return false
		}
	}
	return true
}

func isOneShadow(s string) bool {
	lengths, colors := 0, 0
	for _, tok := range fieldsTop(s) {
		switch {
		case tok == "inset":
		case isLength(tok):
			lengths++
		case isColor(tok):
			colors++
		default:
			return false
		}
	}
	return lengths >= 2 && lengths <= 4 && colors == 1
}

func isFont(v string) bool {
	for _, fam := range strings.Split(v, ",") {
		fam = strings.TrimSpace(fam)
		if !genericFamily[fam] && !familyRe.MatchString(fam) {
			return false
		}
	}
	return true
}
