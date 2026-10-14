package sources

import "slices"

//wails:ignore
func (s *Service) ReferencedGameIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, list := range s.releases {
		for _, r := range list {
			if r.CanonicalGameID != nil && *r.CanonicalGameID != "" {
				seen[*r.CanonicalGameID] = true
			}
			if r.RemoteMatch != nil && r.RemoteMatch.GameID != "" {
				seen[r.RemoteMatch.GameID] = true
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
