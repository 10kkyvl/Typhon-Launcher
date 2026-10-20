package updates

import (
	"slices"

	"typhon/internal/download"
	"typhon/internal/platform"
)

// DownloadInUse reports whether a running update or repair reads from the
// download. When it cannot tell, it says yes: the answer gates a deletion.
//
//wails:ignore
func (s *Service) DownloadInUse(downloadID string) bool {
	s.mu.Lock()
	for gameID := range s.jobs {
		if u := s.updates[gameID]; u != nil && u.DownloadID == downloadID {
			s.mu.Unlock()
			return true
		}
	}
	repairing := s.repairingLocked()
	s.mu.Unlock()
	if len(repairing) == 0 {
		return false
	}
	if s.downloads == nil {
		return true
	}
	for _, gameID := range repairing {
		for _, purpose := range []download.Purpose{download.PurposeRepair, download.PurposeUpdate} {
			for _, task := range s.downloads.ByOrigin(gameID, purpose) {
				if task.ID == downloadID {
					return true
				}
			}
		}
	}
	return false
}

func (s *Service) repairingLocked() []string {
	var ids []string
	for gameID, v := range s.verifications {
		if v.Repairing {
			ids = append(ids, gameID)
		}
	}
	return ids
}

// PathInUse reports whether an update, rollback, swap or repair is working on
// a folder that overlaps path: the install folder of the game, or the folders
// the update derives from it.
//
//wails:ignore
func (s *Service) PathInUse(path string) bool {
	s.mu.Lock()
	var active []string
	for gameID := range s.jobs {
		active = append(active, gameID)
	}
	for gameID, on := range s.rollbackActive {
		if on {
			active = append(active, gameID)
		}
	}
	for gameID := range s.journals {
		active = append(active, gameID)
	}
	active = append(active, s.repairingLocked()...)
	s.mu.Unlock()
	if len(active) == 0 {
		return false
	}
	if s.library == nil {
		return true
	}
	for _, g := range s.library.GetInstalledGames() {
		if !slices.Contains(active, g.ID) {
			continue
		}
		if g.InstallDir == "" {
			return true
		}
		areas := []string{g.InstallDir, g.InstallDir + previousSuffix, g.InstallDir + replacedSuffix}
		if staging, err := stagingDir(g.InstallDir, g.ID); err == nil {
			areas = append(areas, staging)
		}
		for _, area := range areas {
			if platform.SamePath(area, path) || platform.Inside(area, path) || platform.Inside(path, area) {
				return true
			}
		}
	}
	return false
}

// CheckDownloadDeletable is the update side of the delete guard of a download.
//
//wails:ignore
func (s *Service) CheckDownloadDeletable(d download.Download) error {
	if s.DownloadInUse(d.ID) {
		return errDownloadInUse
	}
	root, ok := d.DataRoot()
	if !ok && (d.Flat || d.InPlace) {
		root = d.Destination
	}
	if root != "" && s.PathInUse(root) {
		return errDownloadInUse
	}
	return nil
}
