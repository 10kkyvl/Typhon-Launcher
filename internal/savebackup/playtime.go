package savebackup

import (
	"errors"
	"slices"
	"time"

	"typhon/internal/playlog"
)

var errPlayLogDegraded = errors.New("журнал игровых сессий не сохраняется, наигранное время неполное")

type PlayLog interface {
	Since(t time.Time) []playlog.Session
	Degraded() playlog.DegradedStatus
}

// PlaytimeFrom answers from the play log. A degraded log has dropped sessions
// it failed to save, so every number would be too low: nothing is known then.
// A moment older than the log's retention is unknown for the same reason.
func PlaytimeFrom(l PlayLog, now func() time.Time) PlaytimeFunc {
	return func(gameID string, since []time.Time) ([]Played, error) {
		if l.Degraded().Degraded {
			return nil, errPlayLogDegraded
		}
		out := make([]Played, len(since))
		if len(since) == 0 {
			return out, nil
		}
		oldest := slices.MinFunc(since, func(a, b time.Time) int { return a.Compare(b) })
		var sessions []playlog.Session
		for _, s := range l.Since(oldest) {
			if s.GameID == gameID {
				sessions = append(sessions, s)
			}
		}
		current := now()
		for i, t := range since {
			if current.Sub(t) > playlog.Retention {
				continue
			}
			var total int64
			for _, s := range sessions {
				start := s.StartedAt
				if start.Before(t) {
					start = t
				}
				if s.EndedAt.After(start) {
					total += int64(s.EndedAt.Sub(start).Seconds())
				}
			}
			out[i] = Played{Seconds: total, Known: true}
		}
		return out, nil
	}
}
