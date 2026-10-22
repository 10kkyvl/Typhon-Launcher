package download

import (
	"path/filepath"

	"typhon/internal/diskreason"
	"typhon/internal/platform"
)

const partNotOurs = "файл не принадлежит лаунчеру — удалите вручную"

// PartFileVerdict says whether the .part file at path may be deleted. Only a
// part file of a record that is already finished qualifies: it is what a
// completed download should have promoted or removed. A part file no record
// accounts for may belong to anyone, a browser included, so it is never offered.
//
//wails:ignore
func (m *Manager) PartFileVerdict(path string) (ok bool, code, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.items {
		if !ownsPart(d, path) {
			continue
		}
		switch {
		case d.Status != StatusCompleted:
			return false, diskreason.OwnedByDownload, "файл принадлежит загрузке"
		case d.Seeding:
			return false, diskreason.Seeding, "файлы раздаются"
		}
		return true, "", ""
	}
	return false, diskreason.ForeignPart, partNotOurs
}

func ownsPart(d *Download, path string) bool {
	if root, ok := removalRoot(d); ok && d.Destination != "" &&
		platform.SamePath(filepath.Join(d.Destination, root)+PartFileSuffix, path) {
		return true
	}
	if d.Flat && d.Destination != "" {
		for _, f := range d.Files {
			if isSafeTorrentPath(f.Path) &&
				platform.SamePath(filepath.Join(d.Destination, filepath.FromSlash(f.Path))+PartFileSuffix, path) {
				return true
			}
		}
	}
	return false
}
