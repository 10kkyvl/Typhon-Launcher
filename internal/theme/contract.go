package theme

import (
	"math/big"
	"regexp"
	"strings"
)

// currentContract is the token contract this launcher writes: 1rem is 10px
// and token values follow the 2.0 design. A theme without it was saved by a
// 1.x launcher, where 1rem was 11px.
const currentContract = 2

var (
	remNumber = regexp.MustCompile(`[0-9]*\.?[0-9]+rem`)
	remScale  = big.NewRat(11, 10)
)

// toCurrentContract brings a theme saved under the 1.x contract to the
// current one. A fork stores the whole preset it came from, so values equal
// to that preset as released are taken as never touched and follow the
// preset's current design; every other value is the user's and keeps its
// pixel size. The input and the presets are left unchanged.
func toCurrentContract(t Theme) Theme {
	if t.Contract >= currentContract {
		return t
	}
	parent, released, ok := releasedParent(t)
	if !ok {
		return t
	}
	tokens := make(map[string]string, len(allowedTokens))
	for name, value := range t.Tokens {
		if was, ok := released[name]; ok && was == value {
			if now, ok := parent.Tokens[name]; ok {
				tokens[name] = now
				continue
			}
		}
		tokens[name] = userValueAtCurrentScale(name, value)
	}

	full := true
	for name := range released {
		if _, ok := t.Tokens[name]; !ok {
			full = false
			break
		}
	}
	lastReleased := releasedPresetGenerations[len(releasedPresetGenerations)-1].presets[parent.ID]
	for _, tok := range allowedTokens {
		if _, ok := tokens[tok.Name]; ok {
			continue
		}
		// A partial map leans on the stylesheet for what it leaves out, as it
		// did in 1.x; it only gains tokens that 1.x did not have at all.
		if _, old := lastReleased[tok.Name]; old && !full {
			continue
		}
		if now, ok := parent.Tokens[tok.Name]; ok {
			tokens[tok.Name] = now
		}
	}
	t.Tokens = tokens
	t.Contract = currentContract
	return t
}

// releasedParent finds the built-in preset and the released generation of it
// that the theme was forked from: the one sharing the most values with it.
// A tie goes to the default preset, then to the newer generation.
func releasedParent(t Theme) (Theme, map[string]string, bool) {
	type candidate struct {
		preset    Theme
		tokens    map[string]string
		score     int
		isDefault bool
		release   int
	}
	better := func(a, b candidate) bool {
		if a.score != b.score {
			return a.score > b.score
		}
		if a.isDefault != b.isDefault {
			return a.isDefault
		}
		return a.release > b.release
	}
	var best candidate
	found := false
	for _, p := range presets {
		if p.Base != t.Base {
			continue
		}
		for i, gen := range releasedPresetGenerations {
			tokens, ok := gen.presets[p.ID]
			if !ok {
				continue
			}
			c := candidate{preset: p, tokens: tokens, isDefault: p.ID == defaultTheme().ID, release: i}
			for name, value := range t.Tokens {
				if was, ok := tokens[name]; ok && was == value {
					c.score++
				}
			}
			if !found || better(c, best) {
				best, found = c, true
			}
		}
	}
	return best.preset, best.tokens, found
}

// userValueAtCurrentScale keeps a user's length or shadow at the pixel size
// it had under a 11px rem. A value that would leave the allowed range once
// scaled stays as stored, so the theme can still be saved.
func userValueAtCurrentScale(name, value string) string {
	kind := allowedTokenSet[name]
	if kind != KindLength && kind != KindShadow {
		return value
	}
	scaled := scaleRem(value)
	if scaled == value || validTokenValue(name, kind, value) && !validTokenValue(name, kind, scaled) {
		return value
	}
	return scaled
}

func validTokenValue(name string, kind Kind, value string) bool {
	return validateTokenValue(value) == nil && validTokenKind(name, kind, value)
}

func scaleRem(value string) string {
	matches := remNumber.FindAllStringIndex(value, -1)
	if matches == nil {
		return value
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		before := start
		if before > 0 && value[before-1] == '-' {
			before--
		}
		if before > 0 && isNameByte(value[before-1]) || end < len(value) && isNameByte(value[end]) {
			continue
		}
		digits := value[start : end-len("rem")]
		if digits[0] == '.' {
			digits = "0" + digits
		}
		number, ok := new(big.Rat).SetString(digits)
		if !ok {
			continue
		}
		b.WriteString(value[last:start])
		b.WriteString(decimal(number.Mul(number, remScale)))
		b.WriteString("rem")
		last = end
	}
	b.WriteString(value[last:])
	return b.String()
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.'
}

// decimal writes r with at most four decimals and no trailing zeros.
func decimal(r *big.Rat) string {
	s := r.FloatString(4)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}
