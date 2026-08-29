package updates

import (
	"log/slog"

	"typhon/internal/history"
)

// SetHistoryRecorder wires the persistent action journal. Nil clears it, and
// every call site checks for nil since the service can be constructed
// without one in tests.
//
//wails:ignore
func (s *Service) SetHistoryRecorder(fn func(history.Record) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyRecorder = fn
}

// gameTitle resolves a display title for history records. Falls back to the
// game ID when the library has no title (game already dropped, or library
// not wired in tests), since Record rejects an empty Title.
func (s *Service) gameTitle(gameID string) string {
	if game, ok := s.installedGame(gameID); ok && game.Title != "" {
		return game.Title
	}
	return gameID
}

// recordUpdateHistory writes a terminal update record from the StartUpdate
// background goroutine, which has no caller left to return a journal error
// to; Record already flipped history into Degraded and emitted
// history:degraded, so the user learns about the journal problem from the
// banner instead of an already-terminal update reporting a second failure.
func (s *Service) recordUpdateHistory(r history.Record) {
	if s.historyRecorder == nil {
		return
	}
	if err := s.historyRecorder(r); err != nil {
		slog.Error("record update history", "game", r.GameID, "kind", r.Kind, "error", err)
	}
}
