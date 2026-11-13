package library

import "context"

// StubLaunch makes PlayGame run without an environment or a process: start
// stands for the moment the game's process would start.
func StubLaunch(s *Service, ctx context.Context, start func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	s.prepare = func(context.Context, launch) error { return nil }
	s.start = func(context.Context, launch) (gameProcess, error) { return nil, start() }
}

// SetCanonicalID gives a game the catalog id discovery would give it.
func SetCanonicalID(s *Service, id, canonical string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if s.games[i].ID == id {
			s.games[i].CanonicalGameID = canonical
		}
	}
}
