package library

import (
	"fmt"
	"log/slog"
	"time"

	"typhon/internal/uierr"
)

const StatusMastered = "mastered"

const maxCompletedAtSkew = 24 * time.Hour

var minCompletedAt = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

var (
	errCompletedAtNeedsStatus = uierr.New("library.completed_at_needs_status", "дату прохождения можно задать только у пройденной игры")
	errCompletedAtRequired    = uierr.New("library.completed_at_required", "дата прохождения не указана")
	errCompletedAtTooOld      = uierr.New("library.completed_at_too_old", "дата прохождения раньше 1980 года")
	errCompletedAtInFuture    = uierr.New("library.completed_at_in_future", "дата прохождения в будущем")
)

func IsCompletedStatus(status string) bool {
	return status == StatusCompleted || status == StatusMastered
}

func completedAtAfter(from, to string, current *time.Time, now time.Time) *time.Time {
	switch {
	case !IsCompletedStatus(to):
		return nil
	case IsCompletedStatus(from):
		return current
	default:
		return &now
	}
}

func checkCompletedAt(date, now time.Time) error {
	switch {
	case date.IsZero():
		return errCompletedAtRequired
	case date.Before(minCompletedAt):
		return errCompletedAtTooOld
	case date.After(now.Add(maxCompletedAtSkew)):
		return errCompletedAtInFuture
	default:
		return nil
	}
}

func syncedCompletedAt(status string, date *time.Time, now time.Time) *time.Time {
	if !IsCompletedStatus(status) || date == nil {
		return nil
	}
	if err := checkCompletedAt(*date, now); err != nil {
		slog.Warn("synced completion date dropped", "status", status, "error", err)
		return nil
	}
	t := date.UTC()
	return &t
}

func (s *Service) SetCompletedAt(id string, date time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	if !IsCompletedStatus(game.Status) {
		return errCompletedAtNeedsStatus
	}
	now := s.now()
	if err := checkCompletedAt(date, now); err != nil {
		return err
	}
	// A day picked as "today" arrives as a moment later today; the server
	// clips anything past its clock, and a date it stored differently from ours
	// would never look synced.
	if date.After(now) {
		date = now
	}
	date = date.UTC()
	if game.CompletedAt != nil && game.CompletedAt.Equal(date) {
		return nil
	}
	previous := *game
	game.CompletedAt = &date
	game.StatusAt = &now
	if err := s.persist(); err != nil {
		*game = previous
		return fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return nil
}
