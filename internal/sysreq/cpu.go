package sysreq

import (
	"regexp"
	"strconv"
	"strings"
)

// CPU tier = generation*4 + class, so a newer generation always outranks an
// older one regardless of class (an i5-12400F beats an i9-9900K). Generation
// is the Intel generation number; AMD parts are mapped to the Intel
// generation of comparable per-core speed.

var (
	intelCoreI = regexp.MustCompile(`(?i)\bi([3579])[\s-]*(\d{3,5})`)
	coreUltra  = regexp.MustCompile(`(?i)\bcore\s*(?:\(tm\)\s*)?ultra\s*([579])\b`)
	core2Re    = regexp.MustCompile(`(?i)\bcore\s*(?:\(tm\)\s*)?2\s*(duo|quad|extreme)`)
	oldIntelRe = regexp.MustCompile(`(?i)\b(pentium|celeron|atom)\b`)
	ryzenRe    = regexp.MustCompile(`(?i)\bryzen\s*(?:ai\s*)?([3579])\s*(?:pro\s*)?(\d{4})`)
	fxRe       = regexp.MustCompile(`(?i)\bfx(?:\(tm\))?\W{0,3}(\d)(\d{3})\b`)
	phenomRe   = regexp.MustCompile(`(?i)\bphenom\b`)
	athlonRe   = regexp.MustCompile(`(?i)\b(athlon|sempron)\b`)
	appleRe    = regexp.MustCompile(`(?i)\bapple\s*m(\d)\b`)
)

var classWeight = map[string]int{"3": 0, "5": 1, "7": 2, "9": 3}

var ryzenGen = map[byte]int{'1': 6, '2': 7, '3': 9, '4': 10, '5': 11, '6': 12, '7': 13, '8': 13, '9': 14}

func cpuTier(text string) int {
	if m := intelCoreI.FindStringSubmatch(text); m != nil {
		gen := intelGen(m[2])
		if gen == 0 {
			return 0
		}
		return gen*4 + classWeight[m[1]]
	}
	if m := coreUltra.FindStringSubmatch(text); m != nil {
		return 15*4 + classWeight[m[1]]
	}
	if m := ryzenRe.FindStringSubmatch(text); m != nil {
		gen := ryzenGen[m[2][0]]
		if gen == 0 {
			return 0
		}
		return gen*4 + classWeight[m[1]]
	}
	if m := fxRe.FindStringSubmatch(text); m != nil {
		if m[1] == "4" || m[1] == "6" {
			return 12
		}
		return 13
	}
	if m := core2Re.FindStringSubmatch(text); m != nil {
		if strings.EqualFold(m[1], "duo") {
			return 1
		}
		return 3
	}
	if oldIntelRe.MatchString(text) || athlonRe.MatchString(text) {
		return 1
	}
	if phenomRe.MatchString(text) {
		return 6
	}
	if m := appleRe.FindStringSubmatch(text); m != nil {
		gen, err := strconv.Atoi(m[1])
		if err != nil || gen == 0 {
			return 0
		}
		return (10+gen)*4 + 2
	}
	return 0
}

// intelGen reads the generation from the model number: 3 digits is the first
// generation, 4 digits is 10xx-14xx for generations 10-14 (any suffix,
// 1235U, 1065G7, 1240P) and the first digit otherwise, 5 digits starts with
// the generation. 0 means unreadable.
func intelGen(digits string) int {
	switch len(digits) {
	case 3:
		return 1
	case 4:
		if g, err := strconv.Atoi(digits[:2]); err == nil && g >= 10 && g <= 14 {
			return g
		}
		g, err := strconv.Atoi(digits[:1])
		if err != nil {
			return 0
		}
		return g
	default:
		g, err := strconv.Atoi(digits[:2])
		if err != nil {
			return 0
		}
		return g
	}
}
