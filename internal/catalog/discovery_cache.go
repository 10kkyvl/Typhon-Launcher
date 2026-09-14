package catalog

import "time"

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
	if s.discoveryGames == nil {
		s.discoveryGames = map[string]discoveryGame{}
	}
	for _, item := range items {
		s.discoveryGames[item.Game.ID] = discoveryGame{game: item.Game, used: time.Now()}
	}
	for len(s.discoveryGames) > 256 {
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
