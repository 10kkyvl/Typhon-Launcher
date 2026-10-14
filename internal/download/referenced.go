package download

import "slices"

//wails:ignore
func (m *Manager) ReferencedGameIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	for _, d := range m.items {
		if d.Origin.GameID != "" {
			seen[d.Origin.GameID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
