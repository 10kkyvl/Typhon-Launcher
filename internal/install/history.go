package install

import "typhon/internal/history"

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
