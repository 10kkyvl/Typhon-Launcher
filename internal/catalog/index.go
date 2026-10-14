package catalog

import (
	"slices"
	"sort"
	"strings"

	"typhon/internal/titles"
)

type entry struct {
	normalized string
	aliases    []string
	tokens     []string
	matchable  bool
}

type index struct {
	entries    []entry
	games      []Game
	byID       map[string]int
	byTitle    map[string][]int
	byAlias    map[string][]int
	byToken    map[string][]int
	byExternal map[string]int

	byServer       map[string][]int
	byIGDB         map[string][]int
	bySteam        map[string][]int
	byGOG          map[string][]int
	byIdentity     map[string][]int
	byLocal        map[string][]int
	redirectGroup  map[string][]int
	redirectTarget map[int]int
}

func buildIndex(games []Game) *index {
	idx := &index{
		games:          games,
		entries:        make([]entry, 0, len(games)),
		byID:           make(map[string]int, len(games)),
		byTitle:        make(map[string][]int, len(games)),
		byAlias:        map[string][]int{},
		byToken:        map[string][]int{},
		byExternal:     map[string]int{},
		byServer:       make(map[string][]int, len(games)),
		byIGDB:         make(map[string][]int, len(games)),
		bySteam:        make(map[string][]int, len(games)),
		byGOG:          map[string][]int{},
		byIdentity:     make(map[string][]int, len(games)),
		byLocal:        map[string][]int{},
		redirectGroup:  map[string][]int{},
		redirectTarget: map[int]int{},
	}
	for _, g := range games {
		idx.add(g)
	}
	return idx
}

// matchable отделяет игры от DLC, бандлов и паков. Записи всех типов лежат в
// индексах целиком: каталог, поиск по нему и ручной выбор игры показывают всё,
// что пришло с бэкенда. Тип смотрит только автоматический матчинг (resolve) —
// иначе репак прицепляется к дополнению, которое делит с игрой название.
// Пустой тип считается игрой: у записей, которых бэкенд ещё не переливал, его
// просто нет.
func (idx *index) add(g Game) int {
	pos := len(idx.entries)
	idx.entries = append(idx.entries, entry{})
	idx.registerAt(pos, g)
	return pos
}

// registerAt (re)computes the entry at pos and adds its postings to the
// reverse indexes. The caller owns entries[pos]: add appends a placeholder
// first, update clears the previous postings first via removePostings.
func (idx *index) registerAt(pos int, g Game) {
	normalized := titles.Normalize(g.Title)
	e := entry{
		normalized: normalized,
		tokens:     titles.TokenSet(normalized),
		matchable:  titles.IsGameType(g.GameType),
	}
	for _, alias := range g.Aliases {
		normalizedAlias := titles.Normalize(alias)
		if normalizedAlias == "" || normalizedAlias == normalized {
			continue
		}
		e.aliases = append(e.aliases, normalizedAlias)
	}

	idx.entries[pos] = e
	idx.byID[g.ID] = pos
	if normalized != "" {
		addPosting(idx.byTitle, normalized, pos)
	}
	for _, alias := range e.aliases {
		addPosting(idx.byAlias, alias, pos)
	}
	for _, token := range e.tokens {
		addPosting(idx.byToken, token, pos)
	}
	for _, key := range externalKeys(g.ExternalIDs) {
		if current, ok := idx.byExternal[key]; !ok || pos > current {
			idx.byExternal[key] = pos
		}
	}
	idx.registerRemote(pos, g)
}

// The remote identity maps hold every position sharing a key in ascending
// order, so the winner of a duplicate key does not depend on the order the
// records were registered in: the first position for IGDB and Steam ids, the
// last one for ServerID.
func (idx *index) registerRemote(pos int, g Game) {
	if g.ServerID != "" {
		addPosting(idx.byServer, g.ServerID, pos)
	}
	if g.ExternalIDs.IGDB != "" {
		addPosting(idx.byIGDB, strings.ToLower(g.ExternalIDs.IGDB), pos)
	}
	if g.ExternalIDs.Steam != "" {
		addPosting(idx.bySteam, strings.ToLower(g.ExternalIDs.Steam), pos)
	}
	if g.ExternalIDs.GOG != "" {
		addPosting(idx.byGOG, strings.ToLower(g.ExternalIDs.GOG), pos)
	}
	forEachIdentityKey(g, func(key string) { addPosting(idx.byIdentity, key, pos) })
	forEachLocalIdentityKey(g.LocalExternalIDs, func(key string) { addPosting(idx.byLocal, key, pos) })
}

func (idx *index) unregisterRemote(pos int, g Game) {
	if g.ServerID != "" {
		removePosting(idx.byServer, g.ServerID, pos)
	}
	if g.ExternalIDs.IGDB != "" {
		removePosting(idx.byIGDB, strings.ToLower(g.ExternalIDs.IGDB), pos)
	}
	if g.ExternalIDs.Steam != "" {
		removePosting(idx.bySteam, strings.ToLower(g.ExternalIDs.Steam), pos)
	}
	if g.ExternalIDs.GOG != "" {
		removePosting(idx.byGOG, strings.ToLower(g.ExternalIDs.GOG), pos)
	}
	forEachIdentityKey(g, func(key string) { removePosting(idx.byIdentity, key, pos) })
	forEachLocalIdentityKey(g.LocalExternalIDs, func(key string) { removePosting(idx.byLocal, key, pos) })
}

// matchRemote finds the saved record a remote page item refers to.
func (idx *index) matchRemote(game Game) int {
	if game.ServerID != "" {
		if list := idx.byServer[game.ServerID]; len(list) > 0 {
			return list[len(list)-1]
		}
	}
	if game.ExternalIDs.IGDB != "" {
		if list := idx.byIGDB[strings.ToLower(game.ExternalIDs.IGDB)]; len(list) > 0 {
			return list[0]
		}
	}
	// A Steam-only server row may reuse an existing local Steam record. Once
	// the row has a confirmed IGDB id, a matching Steam id can belong to a
	// different local record and must be reconciled as a provider claim.
	if game.ExternalIDs.IGDB == "" && game.ExternalIDs.Steam != "" {
		if list := idx.bySteam[strings.ToLower(game.ExternalIDs.Steam)]; len(list) > 0 {
			return list[0]
		}
	}
	return -1
}

func addPosting(m map[string][]int, key string, pos int) {
	list := m[key]
	at, found := slices.BinarySearch(list, pos)
	if found {
		return
	}
	m[key] = slices.Insert(list, at, pos)
}

func removePosting(m map[string][]int, key string, pos int) {
	list := m[key]
	at, found := slices.BinarySearch(list, pos)
	if !found {
		return
	}
	if len(list) == 1 {
		delete(m, key)
		return
	}
	m[key] = slices.Delete(slices.Clone(list), at, at+1)
}

func forEachIdentityKey(game Game, fn func(key string)) {
	if id := game.ExternalIDs.IGDB; id != "" {
		fn("igdb:" + strings.ToLower(id))
	}
	if id := game.ExternalIDs.Steam; id != "" {
		fn("steam:" + strings.ToLower(id))
	}
	for _, linked := range game.ProviderLinks["igdb"] {
		if linked != "" {
			fn("igdb:" + strings.ToLower(linked))
		}
	}
	for _, linked := range game.ProviderLinks["steam"] {
		if linked != "" {
			fn("steam:" + strings.ToLower(linked))
		}
	}
}

func forEachLocalIdentityKey(ids ExternalIDs, fn func(key string)) {
	if ids.IGDB != "" {
		fn("igdb:" + strings.ToLower(ids.IGDB))
	}
	if ids.Steam != "" {
		fn("steam:" + strings.ToLower(ids.Steam))
	}
}

// update refreshes the entry at pos after idx.games[pos] was overwritten in
// place with a new value. It only touches the postings that belonged to old,
// instead of rebuilding the whole index — the caller (a durable catalog page
// merge, or a link-reconciliation pass) may touch a handful of positions out
// of tens of thousands and cannot afford buildIndex's full O(n) pass.
func (idx *index) update(pos int, old Game) {
	idx.removePostings(pos, old, idx.entries[pos])
	idx.registerAt(pos, idx.games[pos])
	if target, redirected := idx.redirectTarget[pos]; redirected {
		idx.entries[pos].matchable = false
		idx.byID[idx.games[pos].ID] = target
	}
}

// settleExternal re-points a provider key at the highest position still
// holding that id, which is what a full rebuild would leave there.
func (idx *index) settleExternal(key string) {
	var holders []int
	switch {
	case strings.HasPrefix(key, "igdb:"):
		holders = idx.byIGDB[strings.TrimPrefix(key, "igdb:")]
	case strings.HasPrefix(key, "steam:"):
		holders = idx.bySteam[strings.TrimPrefix(key, "steam:")]
	case strings.HasPrefix(key, "gog:"):
		holders = idx.byGOG[strings.TrimPrefix(key, "gog:")]
	}
	if len(holders) == 0 {
		delete(idx.byExternal, key)
		return
	}
	idx.byExternal[key] = holders[len(holders)-1]
}

func (idx *index) removePostings(pos int, g Game, e entry) {
	if e.normalized != "" {
		idx.byTitle[e.normalized] = removeIntFromSlice(idx.byTitle[e.normalized], pos)
		if len(idx.byTitle[e.normalized]) == 0 {
			delete(idx.byTitle, e.normalized)
		}
	}
	for _, alias := range e.aliases {
		idx.byAlias[alias] = removeIntFromSlice(idx.byAlias[alias], pos)
		if len(idx.byAlias[alias]) == 0 {
			delete(idx.byAlias, alias)
		}
	}
	for _, token := range e.tokens {
		idx.byToken[token] = removeIntFromSlice(idx.byToken[token], pos)
		if len(idx.byToken[token]) == 0 {
			delete(idx.byToken, token)
		}
	}
	idx.unregisterRemote(pos, g)
	for _, key := range externalKeys(g.ExternalIDs) {
		if idx.byExternal[key] == pos {
			idx.settleExternal(key)
		}
	}
}

func removeIntFromSlice(list []int, value int) []int {
	for i, v := range list {
		if v == value {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
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
	return idx.games[pos], true
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
		return idx.games[found[a].pos].SortTitle < idx.games[found[b].pos].SortTitle
	})
	if len(found) > limit {
		found = found[:limit]
	}
	games := make([]Game, 0, len(found))
	for _, item := range found {
		games = append(games, idx.games[item.pos])
	}
	return games
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
