package library

import "fmt"

// GameModeOverride: a nil field inherits the global switch, false switches it off.
type GameModeOverride struct {
	HighPerformance *bool `json:"highPerformance,omitempty"`
	HideLauncher    *bool `json:"hideLauncher,omitempty"`
	RaisePriority   *bool `json:"raisePriority,omitempty"`
}

func (o GameModeOverride) empty() bool {
	return o.HighPerformance == nil && o.HideLauncher == nil && o.RaisePriority == nil
}

func (o GameModeOverride) clone() GameModeOverride {
	return GameModeOverride{
		HighPerformance: cloneBool(o.HighPerformance),
		HideLauncher:    cloneBool(o.HideLauncher),
		RaisePriority:   cloneBool(o.RaisePriority),
	}
}

func cloneBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func (s *Service) SetGameModeOverride(id string, o GameModeOverride) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	previous := *game
	if o.empty() {
		game.GameMode = nil
	} else {
		stored := o.clone()
		game.GameMode = &stored
	}
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

// RunningPID returns the process id a running session is bound to. It takes
// the library lock, so a session watcher must not call it from its callback.
//
//wails:ignore
func (s *Service) RunningPID(id string) (uint32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.running[id]
	if !ok || sess.pid == 0 {
		return 0, false
	}
	return sess.pid, true
}
