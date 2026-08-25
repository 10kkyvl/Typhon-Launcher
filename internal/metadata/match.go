package metadata

import (
	"sort"

	"typhon/internal/titles"
)

const (
	autoThreshold  = 0.90
	ambiguityDelta = 0.05
	yearBonus      = 0.05
	yearPenalty    = 0.25
	yearTolerance  = 1
)

func rank(title string, year int, candidates []Candidate) []Candidate {
	normalized := titles.Normalize(title)
	ranked := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		c.Confidence = confidence(normalized, year, c)
		ranked = append(ranked, c)
	}
	sort.SliceStable(ranked, func(a, b int) bool {
		if ranked[a].Confidence != ranked[b].Confidence {
			return ranked[a].Confidence > ranked[b].Confidence
		}
		return ranked[a].Title < ranked[b].Title
	})
	return ranked
}

func confidence(normalized string, year int, c Candidate) float64 {
	if normalized == "" {
		return 0
	}
	score := titles.Similarity(normalized, titles.Normalize(c.Title))
	if year > 0 && c.ReleaseYear > 0 {
		switch diff := abs(year - c.ReleaseYear); {
		case diff == 0:
			score += yearBonus
		case diff <= yearTolerance:
		default:
			score -= yearPenalty
		}
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func pick(title string, year int, candidates []Candidate) (Candidate, error) {
	ranked := rank(title, year, candidates)
	if len(ranked) == 0 {
		return Candidate{}, ErrNoMatch
	}
	top := ranked[0]
	if top.Confidence < autoThreshold {
		return Candidate{}, ErrAmbiguous
	}
	if len(ranked) > 1 && top.Confidence-ranked[1].Confidence < ambiguityDelta {
		return Candidate{}, ErrAmbiguous
	}
	return top, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
