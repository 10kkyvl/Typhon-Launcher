package install

// Busy reports whether gameID has an installation in a non-terminal status.
// Used by internal/relocate to refuse moving a game while it is still being
// installed, extracted, or waiting on the user.
//
//wails:ignore
func (s *Service) Busy(gameID string) bool {
	if gameID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.GameID == gameID && active(item.Status) {
			return true
		}
	}
	return false
}
