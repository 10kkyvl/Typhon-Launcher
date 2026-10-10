package profile

import (
	"typhon/internal/account"
	"typhon/internal/catalog"
)

type stubCatalog struct {
	games map[string]catalog.Game
}

func (c stubCatalog) GetGames(ids []string) []catalog.Game {
	var out []catalog.Game
	for _, id := range ids {
		if g, ok := c.games[id]; ok {
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
