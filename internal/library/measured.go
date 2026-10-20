package library

import (
	"fmt"
	"log/slog"
)

// UpdateMeasuredSize stores a size taken by a later measurement of the game's
// install folder. The size was measured for installDir; when the game has been
// moved since, the number describes a folder it no longer lives in and is
// dropped. It reports whether the record changed.
//
//wails:ignore
func (s *Service) UpdateMeasuredSize(id, installDir string, size int64, unknown bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := -1
	for i := range s.games {
		if s.games[i].ID == id {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false, errNotFound
	}
	previous := s.games[pos]
	if previous.InstallDir != installDir {
		return false, nil
	}
	if previous.SizeBytes == size && previous.SizeUnknown == unknown {
		return false, nil
	}
	s.games[pos].SizeBytes = size
	s.games[pos].SizeUnknown = unknown
	if err := s.persist(); err != nil {
		s.games[pos] = previous
		return false, fmt.Errorf("save library: %w", err)
	}
	slog.Info("game size updated", "id", id, "bytes", size, "unknown", unknown)
	s.emitUpdated()
	return true, nil
}
