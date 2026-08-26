package catalog

import (
	"sort"

	"typhon/internal/titles"
)

const aliasFloor = 0.6

func sanitize(games []Game) ([]Game, bool) {
	junk := make([][]string, len(games))
	changed := false
	for i := range games {
		kept, dropped := splitAliases(games[i])
		junk[i] = dropped
		if len(dropped) == 0 {
			continue
		}
		games[i].Aliases = kept
		changed = true
	}
	for _, group := range duplicateGroups(games) {
		survivor := pickSurvivor(games, group, junk)
		for _, pos := range group {
			if pos == survivor || len(junk[pos]) == 0 {
				continue
			}
			games[pos] = revert(games[pos], junk[pos][0])
			changed = true
		}
	}
	return games, changed
}

func splitAliases(game Game) (kept, dropped []string) {
	normalized := titles.Normalize(game.Title)
	for _, alias := range game.Aliases {
		if titles.Similarity(alias, normalized) < aliasFloor {
			dropped = append(dropped, alias)
			continue
		}
		kept = append(kept, alias)
	}
	return kept, dropped
}

func duplicateGroups(games []Game) [][]int {
	byProvider := make(map[string][]int, len(games))
	for i := range games {
		id := games[i].ExternalIDs.IGDB
		if id == "" {
			continue
		}
		byProvider[id] = append(byProvider[id], i)
	}
	groups := make([][]int, 0)
	for _, group := range byProvider {
		if len(group) > 1 {
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(a, b int) bool { return groups[a][0] < groups[b][0] })
	return groups
}

func pickSurvivor(games []Game, group []int, junk [][]string) int {
	best := -1
	for _, pos := range group {
		if len(junk[pos]) > 0 {
			continue
		}
		if best == -1 || games[pos].CreatedAt.Before(games[best].CreatedAt) {
			best = pos
		}
	}
	if best != -1 {
		return best
	}
	best = group[0]
	for _, pos := range group {
		if games[pos].CreatedAt.Before(games[best].CreatedAt) {
			best = pos
		}
	}
	return best
}

func revert(game Game, title string) Game {
	game.Title = title
	game.SortTitle = sortTitle(title)
	game.Provisional = true
	game.Aliases = nil
	game.ExternalIDs.IGDB = ""
	game.Summary = ""
	game.Developer = ""
	game.Publisher = ""
	game.Genres = nil
	game.Themes = nil
	game.Platforms = nil
	game.CoverAssetID = ""
	game.HeroAssetID = ""
	game.ReleaseDate = nil
	game.ReleaseYear = nil
	game.MetadataUpdatedAt = nil
	game.MetadataPartial = false
	return game
}
