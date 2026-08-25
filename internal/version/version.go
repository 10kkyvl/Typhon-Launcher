package version

import (
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	KindSemantic      Kind = "semantic"
	KindNumeric       Kind = "numeric"
	KindBuild         Kind = "build"
	KindDate          Kind = "date"
	KindProviderOrder Kind = "provider_order"
	KindUnknown       Kind = "unknown"
)

type Version struct {
	Raw        string `json:"raw"`
	Normalized string `json:"normalized"`
	Kind       Kind   `json:"kind"`
	Series     string `json:"series,omitempty"`
	Parts      []int  `json:"parts,omitempty"`
	BuildID    string `json:"buildId,omitempty"`
	Comparable bool   `json:"comparable"`
}

const (
	minYear  = 1990
	maxYear  = 2100
	maxParts = 6
)

var (
	reSpaceRun  = regexp.MustCompile(`\s+`)
	rePrefix    = regexp.MustCompile(`^(?:v|ver|version)[\s._-]*(?:[0-9])`)
	reBuild     = regexp.MustCompile(`^(?:build|bld|b)[\s._-]*([0-9]{2,})$`)
	reSeries    = regexp.MustCompile(`^(update|upd|patch|hotfix|rev|revision)[\s._-]*([0-9]+(?:[._][0-9]+)*)$`)
	reDotted    = regexp.MustCompile(`^([0-9]+(?:[._-][0-9]+)*)(.*)$`)
	reTrimNoise = regexp.MustCompile(`^[\s._\-()\[\]]+|[\s._\-()\[\]]+$`)
)

var noiseSuffix = map[string]bool{
	"final":    true,
	"release":  true,
	"full":     true,
	"repack":   true,
	"complete": true,
	"gold":     true,
}

func Parse(raw string) Version {
	v := Version{Raw: strings.TrimSpace(raw), Kind: KindUnknown}
	s := strings.ToLower(v.Raw)
	s = reSpaceRun.ReplaceAllString(s, " ")
	s = reTrimNoise.ReplaceAllString(s, "")
	if s == "" {
		return v
	}
	if loc := rePrefix.FindStringIndex(s); loc != nil {
		s = strings.TrimLeft(s[loc[1]-1:], " ._-")
	}
	s = dropNoiseSuffix(s)
	if s == "" {
		return v
	}

	if m := reBuild.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return v
		}
		v.Kind = KindBuild
		v.BuildID = strings.TrimLeft(m[1], "0")
		if v.BuildID == "" {
			v.BuildID = "0"
		}
		v.Parts = []int{n}
		v.Normalized = "build " + v.BuildID
		v.Comparable = true
		return v
	}

	if m := reSeries.FindStringSubmatch(s); m != nil {
		parts, ok := numericParts(m[2])
		if !ok {
			return v
		}
		v.Kind = KindNumeric
		v.Series = seriesName(m[1])
		v.Parts = parts
		v.Normalized = v.Series + " " + joinParts(parts)
		v.Comparable = true
		return v
	}

	if parsed, ok := parseDate(s); ok {
		return parsed
	}

	if m := reDotted.FindStringSubmatch(s); m != nil {
		parts, ok := numericParts(m[1])
		if !ok {
			return v
		}
		v.Parts = parts
		v.Normalized = joinParts(parts)
		if len(parts) >= 3 {
			v.Kind = KindSemantic
		} else {
			v.Kind = KindNumeric
		}
		v.Comparable = strings.TrimLeft(m[2], " ._-") == ""
		if !v.Comparable {
			v.Normalized = v.Normalized + "+" + strings.TrimLeft(m[2], " ._-")
		}
		return v
	}

	v.Normalized = s
	return v
}

func dropNoiseSuffix(s string) string {
	for {
		fields := strings.Fields(s)
		if len(fields) < 2 || !noiseSuffix[fields[len(fields)-1]] {
			return s
		}
		s = strings.Join(fields[:len(fields)-1], " ")
	}
}

func seriesName(raw string) string {
	switch raw {
	case "upd":
		return "update"
	case "rev":
		return "revision"
	default:
		return raw
	}
}

func numericParts(s string) ([]int, bool) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	if len(fields) == 0 || len(fields) > maxParts {
		return nil, false
	}
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		if len(f) > 9 {
			return nil, false
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

func joinParts(parts []int) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strconv.Itoa(p)
	}
	return strings.Join(out, ".")
}

func parseDate(s string) (Version, bool) {
	parts, ok := numericParts(s)
	if !ok || len(parts) != 3 {
		return Version{}, false
	}
	year, month, day := parts[0], parts[1], parts[2]
	if year < minYear || year > maxYear || month < 1 || month > 12 || day < 1 || day > 31 {
		return Version{}, false
	}
	if !strings.ContainsAny(s, ".-_") {
		return Version{}, false
	}
	return Version{
		Raw:        s,
		Normalized: joinParts(parts),
		Kind:       KindDate,
		Parts:      parts,
		Comparable: true,
	}, true
}

func family(k Kind) string {
	switch k {
	case KindSemantic, KindNumeric:
		return "dotted"
	case KindBuild:
		return "build"
	case KindDate:
		return "date"
	default:
		return ""
	}
}

// Key returns a canonical identity for comparable versions so that 1.0 and
// 1.0.0 resolve to the same node when building patch graphs.
func Key(v Version) string {
	if !v.Comparable {
		return ""
	}
	if family(v.Kind) != "dotted" {
		return v.Normalized
	}
	parts := v.Parts
	for len(parts) > 1 && parts[len(parts)-1] == 0 {
		parts = parts[:len(parts)-1]
	}
	if v.Series != "" {
		return v.Series + " " + joinParts(parts)
	}
	return joinParts(parts)
}

func Compare(a, b Version) (int, bool) {
	if !a.Comparable || !b.Comparable {
		return 0, false
	}
	fa, fb := family(a.Kind), family(b.Kind)
	if fa == "" || fa != fb || a.Series != b.Series {
		return 0, false
	}
	return comparePartsOf(a.Parts, b.Parts), true
}

func comparePartsOf(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		left, right := 0, 0
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if left != right {
			if left < right {
				return -1
			}
			return 1
		}
	}
	return 0
}

func Newer(candidate, current Version) (bool, bool) {
	cmp, ok := Compare(candidate, current)
	if !ok {
		return false, false
	}
	return cmp > 0, true
}

func Equal(a, b Version) bool {
	cmp, ok := Compare(a, b)
	return ok && cmp == 0
}

func Display(v Version) string {
	if v.Raw != "" {
		return v.Raw
	}
	return v.Normalized
}
