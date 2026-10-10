package profile

import (
	"typhon/internal/account"
	"typhon/internal/catalog"
)

type stubCatalog struct {
	games   map[string]catalog.Game
	aliases map[string]string
	calls   *[][]string
}

func (c stubCatalog) GetGames(ids []string) []catalog.Game {
	if c.calls != nil {
		*c.calls = append(*c.calls, ids)
	}
	var out []catalog.Game
	seen := map[string]bool{}
	for _, id := range ids {
		if canonical, ok := c.aliases[id]; ok {
			id = canonical
		}
		if g, ok := c.games[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, g)
		}
	}
	return out
}

func (c stubCatalog) GameByIGDB(igdbID string) (catalog.Game, bool) {
	for _, g := range c.games {
		if g.ExternalIDs.IGDB == igdbID {
			return g, true
		}
	}
	return catalog.Game{}, false
}

func (c stubCatalog) IGDBIDOf(id string) string { return c.games[id].ExternalIDs.IGDB }

func noLayout() []account.LayoutBlock { return nil }
