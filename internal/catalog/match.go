package catalog

import (
	"sort"

	"typhon/internal/titles"
)

func (idx *index) resolve(q Query, overrides map[string]string) Match {
	if !q.ExternalIDs.empty() {
		for _, key := range externalKeys(q.ExternalIDs) {
			pos, ok := idx.byExternal[key]
			if !ok || !idx.entries[pos].matchable {
				continue
			}
			return single(idx.games[pos], scoreExternalID, MethodExternalID)
		}
	}
	if q.Normalized == "" {
		return Match{Status: StatusUnmatched, Method: MethodNone}
	}
	if gameID, ok := overrides[q.Normalized]; ok {
		if game, ok := idx.game(gameID); ok {
			return single(game, scoreOverride, MethodOverride)
		}
	}

	best := map[int]Candidate{}
	consider := func(pos int, score float64, method Method) {
		e := idx.entries[pos]
		if !e.matchable {
			return
		}
		g := idx.games[pos]
		score = adjust(score, q, g)
		if current, ok := best[pos]; ok && current.Score >= score {
			return
		}
		best[pos] = Candidate{
			GameID: g.ID,
			Title:  g.Title,
			Year:   g.ReleaseYear,
			Score:  score,
			Method: method,
		}
	}

	for _, pos := range idx.byTitle[q.Normalized] {
		consider(pos, scoreExactTitle, MethodExactTitle)
	}
	for _, pos := range idx.byAlias[q.Normalized] {
		consider(pos, scoreAlias, MethodAlias)
	}
	for _, pos := range idx.candidates(q.Normalized) {
		e := idx.entries[pos]
		if !e.matchable {
			continue
		}
		score := titles.Similarity(q.Normalized, e.normalized)
		for _, alias := range e.aliases {
			score = maxFloat(score, titles.Similarity(q.Normalized, alias))
		}
		if score > scoreFuzzyCap {
			score = scoreFuzzyCap
		}
		if score < ReviewThreshold-0.15 {
			continue
		}
		consider(pos, score, MethodFuzzy)
	}

	if len(best) == 0 {
		return Match{Status: StatusUnmatched, Method: MethodNone}
	}
	candidates := make([]Candidate, 0, len(best))
	for _, c := range best {
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].Score != candidates[b].Score {
			return candidates[a].Score > candidates[b].Score
		}
		return candidates[a].Title < candidates[b].Title
	})
	if len(candidates) > MaxCandidates {
		candidates = candidates[:MaxCandidates]
	}

	top := candidates[0]
	// Ambiguity is judged only against rivals that matched the same way as
	// top. scoreExactTitle (0.98) and scoreFuzzyCap (0.95) are only
	// AmbiguityDelta (0.04) apart, so a sequel whose title hits the catalog
	// literally (exact_title) would otherwise always look "ambiguous" next
	// to its prequel, which is a near-guaranteed decent fuzzy neighbor. A
	// literal title/alias hit does not lose to an approximate one; it can
	// only be contested by another literal hit that scored just as well
	// (e.g. two catalog entries sharing the exact same title).
	rival := -1.0
	for _, c := range candidates[1:] {
		if c.Method == top.Method && c.Score > rival {
			rival = c.Score
		}
	}
	ambiguous := rival >= 0 && top.Score-rival < AmbiguityDelta
	switch {
	case top.Score >= AutoThreshold && !ambiguous && top.Method != MethodFuzzy:
		return Match{Status: StatusMatched, GameID: top.GameID, Confidence: top.Score, Method: top.Method, Candidates: candidates}
	case top.Score >= ReviewThreshold:
		return Match{Status: StatusReview, Confidence: top.Score, Method: top.Method, Candidates: candidates}
	default:
		return Match{Status: StatusUnmatched, Confidence: top.Score, Method: MethodNone, Candidates: candidates}
	}
}

func single(game Game, score float64, method Method) Match {
	candidate := Candidate{GameID: game.ID, Title: game.Title, Year: game.ReleaseYear, Score: score, Method: method}
	return Match{
		Status:     StatusMatched,
		GameID:     game.ID,
		Confidence: score,
		Method:     method,
		Candidates: []Candidate{candidate},
	}
}

func adjust(score float64, q Query, game Game) float64 {
	if q.Year > 0 && game.ReleaseYear != nil {
		if *game.ReleaseYear == q.Year {
			score += yearBonus
		} else {
			score -= yearPenalty
		}
	}
	if q.Developer != "" && game.Developer != "" && titles.Normalize(q.Developer) == titles.Normalize(game.Developer) {
		score += developerBonus
	}
	if score > 1 {
		score = 1
	}
	if score < 0 {
		score = 0
	}
	return score
}

// The visited-page cache cannot establish that an official title is unique.
func conservativeMatch(m Match, partial bool) Match {
	if partial && m.Status == StatusMatched && m.Method != MethodExternalID && m.Method != MethodOverride {
		m.Status = StatusReview
		m.GameID = ""
	}
	return m
}
