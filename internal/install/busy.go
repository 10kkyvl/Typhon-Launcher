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

// WorkRunning reports whether an install or a removal is running now: the
// work the launcher's shutdown would wait for or cut short. The self-update
// asks it before it quits the launcher. A pre-elevated broker counts only
// while it runs an installer, and then through that install's job: an idle
// one waits out a whole download and stops within seconds at shutdown.
//
//wails:ignore
func (s *Service) WorkRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) > 0 || s.removing > 0 {
		return true
	}
	for _, item := range s.items {
		if transient(item.Status) {
			return true
		}
	}
	return false
}
