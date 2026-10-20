package gamemode

import (
	"errors"
	"fmt"

	"typhon/internal/uierr"
)

func (s *Service) saveMarker(gameID, toggle string) error {
	if blocked := s.blockedErr(); blocked != nil {
		return blocked
	}
	if err := s.store.save(s.mark); err != nil {
		e := uierr.Wrap(CodeMarkerWriteFailed, err)
		s.report(gameID, toggle, e)
		return e
	}
	return nil
}

// saveMarkerQuiet is for rollbacks: saveMarker has already reported a failure.
func (s *Service) saveMarkerQuiet(gameID, toggle string) {
	if err := s.saveMarker(gameID, toggle); err != nil {
		return
	}
}

func (s *Service) acquirePower(gameID string) bool {
	if s.counts.power == 0 {
		prev, err := s.deps.Power.Active()
		if err != nil {
			s.report(gameID, "power", uierr.Wrap(CodePowerFailed, fmt.Errorf("read active power plan: %w", err)))
			return false
		}
		has, err := s.deps.Power.Has(HighPerformanceScheme)
		if err != nil {
			s.report(gameID, "power", uierr.Wrap(CodePowerFailed, fmt.Errorf("list power plans: %w", err)))
			return false
		}
		if !has {
			s.report(gameID, "power", uierr.New(CodePowerSchemeMissing, "the High performance power plan does not exist on this computer"))
			return false
		}
		if prev == HighPerformanceScheme {
			s.counts.power++
			return true
		}
		s.mark.Power = &powerMark{Previous: prev, Applied: HighPerformanceScheme}
		if s.saveMarker(gameID, "power") != nil {
			s.mark.Power = nil
			return false
		}
		if err := s.deps.Power.Set(HighPerformanceScheme); err != nil {
			s.mark.Power = nil
			s.saveMarkerQuiet(gameID, "power")
			s.report(gameID, "power", uierr.Wrap(CodePowerFailed, fmt.Errorf("switch power plan: %w", err)))
			return false
		}
	}
	s.counts.power++
	return true
}

func (s *Service) releasePower(gameID string) {
	s.counts.power--
	if s.counts.power > 0 {
		return
	}
	if err := s.restorePowerLocked(gameID); err != nil {
		return
	}
	s.saveMarkerQuiet(gameID, "power")
}

// A plan the user deleted meanwhile cannot be restored ever, so it is reported
// once and dropped instead of being retried on every start.
func (s *Service) restorePowerLocked(gameID string) error {
	m := s.mark.Power
	if m == nil {
		return nil
	}
	cur, err := s.deps.Power.Active()
	if err != nil {
		e := uierr.Wrap(CodeRestoreFailed, fmt.Errorf("read active power plan: %w", err))
		s.report(gameID, "power", e)
		return e
	}
	if cur == m.Applied {
		if err := s.deps.Power.Set(m.Previous); err != nil {
			has, hasErr := s.deps.Power.Has(m.Previous)
			if hasErr == nil && !has {
				s.report(gameID, "power", uierr.Wrap(CodeRestoreFailed, fmt.Errorf("the previous power plan %s no longer exists: %w", m.Previous, err)))
				s.mark.Power = nil
				return nil
			}
			e := uierr.Wrap(CodeRestoreFailed, fmt.Errorf("restore power plan: %w", err))
			s.report(gameID, "power", e)
			return e
		}
	}
	s.mark.Power = nil
	return nil
}

func (s *Service) acquireHide(gameID string) bool {
	if s.counts.hide == 0 {
		visible, ok := s.windowVisible()
		if ok && visible {
			s.mark.Hide = true
			if s.saveMarker(gameID, "hide") != nil {
				s.mark.Hide = false
				return false
			}
			s.hidden = s.window(s.deps.Window.Hide)
		}
	}
	s.counts.hide++
	return true
}

func (s *Service) releaseHide(gameID string) {
	s.counts.hide--
	if s.counts.hide > 0 {
		return
	}
	if s.hidden {
		visible, ok := s.windowVisible()
		if ok && !visible {
			s.window(s.deps.Window.Show)
		}
		s.hidden = false
	}
	s.mark.Hide = false
	s.saveMarkerQuiet(gameID, "hide")
}

// restoreAllLocked leaves a marker it could not clear in place for the next
// start, and touches nothing while the marker on disk is unreadable.
func (s *Service) restoreAllLocked(gameID string) error {
	if s.isBlocked() {
		return nil
	}
	var errs []error
	if err := s.restorePowerLocked(gameID); err != nil {
		errs = append(errs, err)
	}
	s.mark.Hide = false
	s.counts = toggles{}
	if err := s.saveMarker(gameID, "marker"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
