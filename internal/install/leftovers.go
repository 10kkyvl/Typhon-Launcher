package install

import (
	"fmt"

	"typhon/internal/diskreason"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/uierr"
)

// rootExists is a var so a test can look at what the caller holds while the
// disk is being read.
var rootExists = pathExists

var errPartialRefused = uierr.New("install.partial_refused", "эту временную папку удалять нельзя")

// PartialVerdict answers whether the folder at path is an abandoned .partial
// of an installation and may be deleted. Every doubt is a refusal: a
// ModeMove install consumed its source into the partial, so the partial can be
// the only copy, and a folder no record explains is not provably ours. The
// code is one of the diskreason values, the reason is text for logs.
//
//wails:ignore
func (s *Service) PartialVerdict(path string) (ok bool, code, reason string) {
	return s.decidePartial(path, nil)
}

// decidePartial reads the disk outside s.mu and decides under it: a source
// folder that has not been looked at yet is stat-ed with the lock released and
// the decision is taken again, so a record that changed meanwhile is judged on
// what it says now. onOK runs under the lock that took the decision.
func (s *Service) decidePartial(path string, onOK func()) (ok bool, code, reason string) {
	exists := map[string]bool{}
	for attempt := 0; attempt < 4; attempt++ {
		s.mu.Lock()
		ok, code, reason, unseen := s.partialVerdictLocked(path, exists)
		if len(unseen) == 0 {
			if ok && onOK != nil {
				onOK()
			}
			s.mu.Unlock()
			return ok, code, reason
		}
		s.mu.Unlock()
		for _, root := range unseen {
			exists[root] = rootExists(root)
		}
	}
	return false, diskreason.Unknown, "состояние установок меняется, повторите позже"
}

func (s *Service) partialVerdictLocked(path string, exists map[string]bool) (ok bool, code, reason string, unseen []string) {
	found := false
	for _, item := range s.items {
		partial := partialPath(item)
		if partial == "" || !samePath(partial, path) {
			continue
		}
		found = true
		if active(item.Status) {
			return false, diskreason.OwnedByInstall, "идёт установка", nil
		}
		if item.Mode != ModeMove {
			continue
		}
		present, seen := exists[item.ContentRoot]
		if !seen {
			unseen = append(unseen, item.ContentRoot)
			continue
		}
		if !present {
			return false, diskreason.OnlyCopy, "единственная копия данных: исходная папка уже перенесена сюда", nil
		}
	}
	if len(unseen) > 0 {
		return false, "", "", unseen
	}
	if !found {
		return false, diskreason.NoRecord, "нет записи об установке, удалять вручную", nil
	}
	return true, "", "", nil
}

// ReservePartial checks that the .partial folder at path may go and, in the
// same step, bars every install from being registered on top of it until
// release is called. The caller deletes between the two.
//
//wails:ignore
func (s *Service) ReservePartial(path string) (func(), error) {
	key, err := platform.PathKey(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errPartialRefused, err)
	}
	ok, _, reason := s.decidePartial(path, func() {
		if s.reservedPartials == nil {
			s.reservedPartials = map[string]int{}
		}
		s.reservedPartials[key]++
	})
	if !ok {
		return nil, fmt.Errorf("%w: %s", errPartialRefused, reason)
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.reservedPartials[key]--; s.reservedPartials[key] <= 0 {
			delete(s.reservedPartials, key)
		}
	}, nil
}

func (s *Service) partialReservedLocked(path string) bool {
	key, err := platform.PathKey(path)
	if err != nil {
		return false
	}
	return s.reservedPartials[key] > 0
}

// PathInUse reports whether a running install works in or around path, or a
// deletion has reserved a .partial there.
//
//wails:ignore
func (s *Service) PathInUse(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if !active(item.Status) {
			continue
		}
		for _, p := range []string{item.Destination, item.SourcePath, item.ContentRoot, partialPath(item)} {
			if sharesPath(path, p) {
				return true
			}
		}
	}
	for key := range s.reservedPartials {
		if sharesPath(path, key) {
			return true
		}
	}
	return false
}

// GameBusy reports whether an install or update job works on the game's folder.
//
//wails:ignore
func (s *Service) GameBusy(game library.Game) bool {
	return s.gameBusy(game)
}
