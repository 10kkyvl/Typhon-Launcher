package sysreq

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	mib = uint64(1) << 20
	gib = uint64(1) << 30
	tib = uint64(1) << 40
)

var sizeRe = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(tb|тб|gb|гб|gigabytes?|гигабайт\w*|mb|мб|megabytes?|мегабайт\w*)`)

func unitBytes(unit string) uint64 {
	switch {
	case strings.HasPrefix(unit, "t"), strings.HasPrefix(unit, "т"):
		return tib
	case strings.HasPrefix(unit, "g"), strings.HasPrefix(unit, "г"):
		return gib
	default:
		return mib
	}
}

func toBytes(num, unit string) (uint64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", "."), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return uint64(v * float64(unitBytes(strings.ToLower(unit)))), true
}

type sizeAt struct {
	bytes uint64
	start int
	end   int
}

func findSizes(text string) []sizeAt {
	var out []sizeAt
	for _, m := range sizeRe.FindAllStringSubmatchIndex(text, -1) {
		b, ok := toBytes(text[m[2]:m[3]], text[m[4]:m[5]])
		if !ok {
			continue
		}
		out = append(out, sizeAt{bytes: b, start: m[0], end: m[1]})
	}
	return out
}

func firstSize(text string) (uint64, bool) {
	sizes := findSizes(text)
	if len(sizes) == 0 {
		return 0, false
	}
	return sizes[0].bytes, true
}

func formatBytes(b uint64) string {
	switch {
	case b >= tib:
		return trimFloat(float64(b)/float64(tib)) + " TB"
	case b >= gib:
		return trimFloat(float64(b)/float64(gib)) + " GB"
	default:
		return trimFloat(float64(b)/float64(mib)) + " MB"
	}
}

func trimFloat(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}
