package install

import "slices"

//wails:ignore
func (s *Service) ReferencedGameIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, item := range s.items {
		if item.Origin.GameID != "" {
			seen[item.Origin.GameID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
