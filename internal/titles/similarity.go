package titles

import (
	"sort"
	"strings"
)

const maxLevInput = 256

func TokenSet(normalized string) []string {
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func Similarity(a, b string) float64 {
	na := Normalize(a)
	nb := Normalize(b)
	if na == nb {
		return 1
	}

	setA := TokenSet(na)
	setB := TokenSet(nb)

	tokenSim := overlapCoefficient(setA, setB)
	levSim := levenshteinSimilarity(na, nb)

	score := 0.5*tokenSim + 0.5*levSim
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

func overlapCoefficient(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, w := range a {
		set[w] = struct{}{}
	}
	inter := 0
	for _, w := range b {
		if _, ok := set[w]; ok {
			inter++
		}
	}
	min := len(a)
	if len(b) < min {
		min = len(b)
	}
	return float64(inter) / float64(min)
}

func levenshteinSimilarity(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) > maxLevInput {
		ra = ra[:maxLevInput]
	}
	if len(rb) > maxLevInput {
		rb = rb[:maxLevInput]
	}

	dist := levenshtein(ra, rb)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(dist)/float64(maxLen)
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost

			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}
