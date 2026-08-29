package download

import "typhon/internal/history"

// SetHistoryRecorder wires the persistent action journal. Nil clears it, and
// every call site checks for nil since the manager can be constructed
// without one in tests.
//
//wails:ignore
func (m *Manager) SetHistoryRecorder(fn func(history.Record) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.historyRecorder = fn
}
