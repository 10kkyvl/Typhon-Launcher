package library

import "slices"

//wails:ignore
func (s *Service) ReferencedCatalogIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, list := range [][]Game{s.games, s.archived} {
		for _, g := range list {
			if g.CanonicalGameID != "" {
				seen[g.CanonicalGameID] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
