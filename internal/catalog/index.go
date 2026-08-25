package catalog

import (
	"sort"
	"strings"

	"typhon/internal/titles"
)

type entry struct {
	game       Game
	normalized string
	aliases    []string
	tokens     []string
}

type index struct {
	entries    []entry
	byID       map[string]int
	byTitle    map[string][]int
	byAlias    map[string][]int
	byToken    map[string][]int
	byExternal map[string]int
}

func buildIndex(games []Game) *index {
	idx := &index{
		entries:    make([]entry, 0, len(games)),
		byID:       make(map[string]int, len(games)),
		byTitle:    make(map[string][]int, len(games)),
		byAlias:    map[string][]int{},
		byToken:    map[string][]int{},
		byExternal: map[string]int{},
	}
	for _, g := range games {
		idx.add(g)
	}
	return idx
}

func (idx *index) add(g Game) int {
	normalized := titles.Normalize(g.Title)
	e := entry{
		game:       g,
		normalized: normalized,
		tokens:     titles.TokenSet(normalized),
	}
	for _, alias := range g.Aliases {
		normalizedAlias := titles.Normalize(alias)
		if normalizedAlias == "" || normalizedAlias == normalized {
			continue
		}
		e.aliases = append(e.aliases, normalizedAlias)
	}

	pos := len(idx.entries)
	idx.entries = append(idx.entries, e)
	idx.byID[g.ID] = pos
	if normalized != "" {
		idx.byTitle[normalized] = append(idx.byTitle[normalized], pos)
	}
	for _, alias := range e.aliases {
		idx.byAlias[alias] = append(idx.byAlias[alias], pos)
	}
	for _, token := range e.tokens {
		idx.byToken[token] = append(idx.byToken[token], pos)
	}
	for _, key := range externalKeys(g.ExternalIDs) {
		idx.byExternal[key] = pos
	}
	return pos
}

func externalKeys(ids ExternalIDs) []string {
	var keys []string
	if ids.Steam != "" {
		keys = append(keys, "steam:"+strings.ToLower(ids.Steam))
	}
	if ids.IGDB != "" {
		keys = append(keys, "igdb:"+strings.ToLower(ids.IGDB))
	}
	if ids.GOG != "" {
		keys = append(keys, "gog:"+strings.ToLower(ids.GOG))
	}
	return keys
}

func (idx *index) game(id string) (Game, bool) {
	pos, ok := idx.byID[id]
	if !ok {
		return Game{}, false
	}
	return idx.entries[pos].game, true
}

func (idx *index) candidates(normalized string) []int {
	tokens := titles.TokenSet(normalized)
	if len(tokens) == 0 {
		return nil
	}
	limit := len(idx.entries) / 10
	if limit < minCommonTokenDoc {
		limit = minCommonTokenDoc
	}

	shared := map[int]int{}
	collect := func(skipCommon bool) {
		for _, token := range tokens {
			postings := idx.byToken[token]
			if skipCommon && len(postings) > limit {
				continue
			}
			for _, pos := range postings {
				shared[pos]++
			}
		}
	}
	collect(true)
	if len(shared) == 0 {
		collect(false)
	}

	pool := make([]int, 0, len(shared))
	for pos := range shared {
		pool = append(pool, pos)
	}
	sort.Slice(pool, func(a, b int) bool {
		if shared[pool[a]] != shared[pool[b]] {
			return shared[pool[a]] > shared[pool[b]]
		}
		return pool[a] < pool[b]
	})
	if len(pool) > candidatePool {
		pool = pool[:candidatePool]
	}
	return pool
}

func (idx *index) search(query string, limit int) []Game {
	normalized := titles.Normalize(query)
	if normalized == "" || limit <= 0 {
		return nil
	}
	type scored struct {
		pos   int
		score float64
	}
	seen := map[int]bool{}
	var found []scored
	for _, pos := range idx.byTitle[normalized] {
		seen[pos] = true
		found = append(found, scored{pos, 1})
	}
	for _, pos := range idx.candidates(normalized) {
		if seen[pos] {
			continue
		}
		e := idx.entries[pos]
		score := titles.Similarity(normalized, e.normalized)
		if strings.Contains(e.normalized, normalized) {
			score = maxFloat(score, 0.9)
		}
		for _, alias := range e.aliases {
			score = maxFloat(score, titles.Similarity(normalized, alias))
		}
		if score < 0.35 {
			continue
		}
		seen[pos] = true
		found = append(found, scored{pos, score})
	}
	sort.Slice(found, func(a, b int) bool {
		if found[a].score != found[b].score {
			return found[a].score > found[b].score
		}
		return idx.entries[found[a].pos].game.SortTitle < idx.entries[found[b].pos].game.SortTitle
	})
	if len(found) > limit {
		found = found[:limit]
	}
	games := make([]Game, 0, len(found))
	for _, item := range found {
		games = append(games, idx.entries[item.pos].game)
	}
	return games
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
