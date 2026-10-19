package library

import (
	"errors"
	"fmt"
)

// Wallpaper Engine живёт весь день, поэтому детект-сессию не-игры закрываем
// здесь, без наигранного времени и playlog, иначе запись не удалить никогда.
//
//wails:ignore
func (s *Service) RemoveExternal(refs []ExternalRef) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	var removed []string
	var errs []error
	for _, ref := range refs {
		pos := s.externalIndexLocked(ref.Provider, ref.AppID)
		if pos < 0 {
			continue
		}
		id := s.games[pos].ID
		if s.starting[id] != nil {
			continue
		}
		sess := s.running[id]
		if sess != nil && (!sess.external || sess.process != nil) {
			continue
		}
		if err := s.removeGameLocked(id); err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", ref.Provider, ref.AppID, err))
			continue
		}
		if sess != nil {
			delete(s.running, id)
			for _, w := range s.watchers {
				w.SessionStopped(id)
			}
			emitStopped(SessionEvent{GameID: id})
		}
		removed = append(removed, id)
	}
	ctx := s.ctx
	s.mu.Unlock()
	for _, id := range removed {
		s.dropSteam(ctx, id)
	}
	return len(removed), errors.Join(errs...)
}
