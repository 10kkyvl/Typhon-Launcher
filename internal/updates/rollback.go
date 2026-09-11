package updates

import (
	"fmt"
	"os"

	"typhon/internal/library"
)

// HasRollback is used by relocation: moving only the current directory would
// strand its previous version and its absolute metadata at the old location.
//
//wails:ignore
func (s *Service) HasRollback(gameID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rollbacks[gameID] != nil || s.journals[gameID] != nil
}

// finishRollback is idempotent at every filesystem and metadata boundary.
// The journal survives until both the library and rollback state are saved.
func (s *Service) finishRollback(j SwapJournal) error {
	r := j.Rollback
	if r == nil || j.InstallDir == "" || j.Staging == "" || j.Previous == "" {
		return fmt.Errorf("%w: incomplete rollback journal", errSwapFailed)
	}
	if exists(j.Staging) {
		if exists(j.InstallDir) {
			if exists(j.Previous) {
				return fmt.Errorf("%w: ambiguous rollback directories", errSwapFailed)
			}
			if err := os.Rename(j.InstallDir, j.Previous); err != nil {
				return err
			}
		}
		if err := os.Rename(j.Staging, j.InstallDir); err != nil {
			return err
		}
	} else if !exists(j.InstallDir) {
		return fmt.Errorf("%w: rollback files missing", errSwapFailed)
	}
	if s.library == nil {
		return errNoLibrary
	}
	if _, err := s.library.ApplyInstalledUpdate(library.InstalledUpdate{
		ID: j.GameID, InstallDir: r.InstallDir, Executable: r.Executable,
		Version: r.Version, VersionSource: string(VersionSourceRelease),
		ReleaseID: r.ReleaseID, SourceID: r.SourceID, DistributionID: r.DistributionID,
		ReleaseUploadedAt: r.ReleaseUploadedAt,
	}); err != nil {
		return err
	}
	s.mu.Lock()
	old, had := s.rollbacks[j.GameID]
	delete(s.rollbacks, j.GameID)
	if err := s.persistRollbacksLocked(); err != nil {
		if had {
			s.rollbacks[j.GameID] = old
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if err := s.clearJournal(j.GameID); err != nil {
		return err
	}
	removeTree(j.Previous)
	s.updateFieldsBestEffort(j.GameID, func(u *Update) { u.CanRollback = false; u.State = StateIdle; u.Plan = nil; u.Error = "" })
	s.store.removeManifest(j.GameID)
	return nil
}

func (s *Service) restoreSwapRollbackMetadata(j SwapJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.rollbacks[j.GameID]
	if j.PreviousRollback != nil {
		copyEntry := *j.PreviousRollback
		s.rollbacks[j.GameID] = &copyEntry
	} else {
		delete(s.rollbacks, j.GameID)
	}
	if err := s.persistRollbacksLocked(); err != nil {
		if had {
			s.rollbacks[j.GameID] = old
		} else {
			delete(s.rollbacks, j.GameID)
		}
		return err
	}
	return nil
}
