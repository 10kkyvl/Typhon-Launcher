package catalog

import "time"

// maxDiscoveryGamesCache bounds the shared temporary cache used by both
// discovery previews and durable catalog pages for games the user has only
// glanced at (never opened). A discovery shelf refreshes a handful of items
// at a time, but browsing the catalog can flip through dozens of 60-item
// pages in one sitting; the cap must cover that without growing unbounded.
const maxDiscoveryGamesCache = 3000

type discoveryGame struct {
	game Game
	used time.Time
}

// Candidate pages are previews: reconcile IDs for ranking without modifying
// personal records or competing with the durable offline page cache.
func (s *Service) previewRemotePage(page GamePage) GamePage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page.Items = append([]Game(nil), page.Items...)
	byServer, byIGDB, bySteam := remoteIndexes(s.games)
	complete := remoteProviderCompleteness(page.Providers)
	for i, g := range page.Items {
		g.LocalExternalIDs = ExternalIDs{}
		g.ServerID = g.ID
		g.Genres = canonicalGenres(g.Genres)
		if pos := remoteMatchPosition(g, byServer, byIGDB, bySteam); pos >= 0 {
			old := s.games[pos]
			g.ID = old.ID
			g = mergeRemoteGame(old, g, complete)
		}
		g.LocalExternalIDs = ExternalIDs{}
		page.Items[i] = g
	}
	aliases := s.remotePageAliasesLocked(page.Items)
	for i := range page.Items {
		page.Items[i].AliasIDs = aliases[i]
	}
	return page
}

func (s *Service) rememberDiscoveryGames(items []RecommendationItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, item := range items {
		s.rememberBrowsedGameLocked(item.Game, now)
	}
}

// rememberBrowsedGameLocked stashes a game the user has only seen in a
// listing — a discovery shelf or a durable catalog page — without writing it
// to s.games/catalog.json. It becomes durable only through
// promoteDiscoveryGameLocked, triggered by the first interaction that needs
// a stable ID (GetGame). Caller holds s.mu.
func (s *Service) rememberBrowsedGameLocked(game Game, used time.Time) {
	if s.discoveryGames == nil {
		s.discoveryGames = map[string]discoveryGame{}
	}
	s.discoveryGames[game.ID] = discoveryGame{game: game, used: used}
	for len(s.discoveryGames) > maxDiscoveryGamesCache {
		oldest := ""
		for id, g := range s.discoveryGames {
			if oldest == "" || g.used.Before(s.discoveryGames[oldest].used) {
				oldest = id
			}
		}
		delete(s.discoveryGames, oldest)
	}
}

// Opening a pick is the first durable interaction. Persist it before metadata
// or installation services use the catalog ID; failed saves leave no mutation.
func (s *Service) promoteDiscoveryGameLocked(id string) (string, error) {
	if _, ok := s.idx.game(id); ok {
		return id, nil
	}
	cached, ok := s.discoveryGames[id]
	if !ok {
		return id, nil
	}
	game := cached.game
	byServer, byIGDB, bySteam := remoteIndexes(s.games)
	if pos := remoteMatchPosition(game, byServer, byIGDB, bySteam); pos >= 0 {
		return s.games[pos].ID, nil
	}
	game.AliasIDs = nil
	previous := append([]Game(nil), s.games...)
	s.games = append(s.games, game)
	s.rebuildLocked()
	if err := s.persistGamesLocked(); err != nil {
		s.games = previous
		s.rebuildLocked()
		return id, err
	}
	delete(s.discoveryGames, id)
	return id, nil
}
