package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"typhon/internal/diskreason"
	"typhon/internal/platform"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var (
	errCleanupUnknown = uierr.New("download.cleanup_unknown", "такой записи об оставшихся файлах нет")
	errCleanupInUse   = uierr.New("download.cleanup_in_use", "эти файлы теперь принадлежат загрузке")
	errCleanupUnsafe  = uierr.New("download.cleanup_unsafe", "путь является ссылкой, удалять его нельзя")
	errCleanupClaimed = uierr.New("download.cleanup_claimed", "в эту папку сейчас удаляют оставшиеся файлы")
)

// Cleanup is a path whose download record was deleted before its files were.
// It is written to disk before the record goes, so a failed or interrupted
// removal never leaves files nobody knows about.
type Cleanup struct {
	Path       string    `json:"path"`
	At         time.Time `json:"at"`
	DownloadID string    `json:"downloadId,omitempty"`
}

func (s *store) cleanupPath() string {
	return filepath.Join(s.dir, "cleanup.json")
}

func (s *store) loadCleanups() ([]Cleanup, error) {
	if s.dir == "" {
		return nil, errors.New("downloads path unavailable")
	}
	data, err := os.ReadFile(s.cleanupPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.cleanupPath(), err)
	}
	var list []Cleanup
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.cleanupPath(), err)
	}
	return list, nil
}

func (s *store) saveCleanups(list []Cleanup) error {
	if s.dir == "" {
		return errors.New("downloads path unavailable")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.cleanupPath(), data)
}

func cleanupPaths(destination, root string, flat bool, files []FileState) []string {
	if destination == "" {
		return nil
	}
	if !flat {
		base := filepath.Join(destination, root)
		return []string{base, base + PartFileSuffix}
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if isSafeTorrentPath(f.Path) {
			out = append(out, filepath.Join(destination, filepath.FromSlash(f.Path)))
		}
	}
	return out
}

func (m *Manager) saveCleanupsLocked() error {
	if err := m.store.saveCleanups(m.cleanups); err != nil {
		return fmt.Errorf("persist cleanup list: %w", err)
	}
	return nil
}

func (m *Manager) addCleanupLocked(downloadID string, paths []string) error {
	if m.cleanupErr != nil {
		return fmt.Errorf("список оставшихся файлов не прочитан: %w", m.cleanupErr)
	}
	previous := slices.Clone(m.cleanups)
	now := time.Now()
	for _, p := range paths {
		m.cleanups = append(m.cleanups, Cleanup{Path: p, At: now, DownloadID: downloadID})
	}
	if err := m.saveCleanupsLocked(); err != nil {
		m.cleanups = previous
		return err
	}
	return nil
}

func (m *Manager) removeCleanupLocked(paths []string) error {
	previous := slices.Clone(m.cleanups)
	m.cleanups = slices.DeleteFunc(m.cleanups, func(c Cleanup) bool {
		return slices.ContainsFunc(paths, func(p string) bool { return platform.SamePath(p, c.Path) })
	})
	if len(m.cleanups) == len(previous) {
		return nil
	}
	if err := m.saveCleanupsLocked(); err != nil {
		m.cleanups = previous
		return err
	}
	return nil
}

// pruneCleanupIDLocked forgets what a download was going to leave behind: the
// download is being removed with its files kept, so they are not leftovers.
func (m *Manager) pruneCleanupIDLocked(downloadID string) error {
	previous := slices.Clone(m.cleanups)
	m.cleanups = slices.DeleteFunc(m.cleanups, func(c Cleanup) bool { return c.DownloadID == downloadID })
	if len(m.cleanups) == len(previous) {
		return nil
	}
	if err := m.saveCleanupsLocked(); err != nil {
		m.cleanups = previous
		return err
	}
	return nil
}

func (m *Manager) dropCleanup(paths []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeCleanupLocked(paths)
}

// PendingCleanups lists the files that a deleted download left on disk.
//
//wails:ignore
func (m *Manager) PendingCleanups() []Cleanup {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.cleanups)
}

func overlaps(a, b string) bool {
	return platform.SamePath(a, b) || platform.Inside(a, b) || platform.Inside(b, a)
}

// areaOf is the part of the disk a download writes into. A download whose
// folder is not known yet, a flat one without a file list and an in-place one
// are answerable only for their whole destination.
func areaOf(d *Download) (string, bool) {
	if d.Destination == "" {
		return "", false
	}
	if root, ok := d.DataRoot(); ok {
		return root, true
	}
	return d.Destination, true
}

// coveredLocked reports whether a current record claims the path or something
// inside or around it.
func (m *Manager) coveredLocked(path string) bool {
	for _, d := range m.items {
		if d.Flat && len(d.Files) > 0 && d.Destination != "" {
			for _, f := range d.Files {
				if isSafeTorrentPath(f.Path) && overlaps(filepath.Join(d.Destination, filepath.FromSlash(f.Path)), path) {
					return true
				}
			}
			continue
		}
		area, ok := areaOf(d)
		if !ok {
			continue
		}
		if overlaps(area, path) || platform.SamePath(area+PartFileSuffix, path) {
			return true
		}
	}
	return false
}

// claimedLocked reports whether a leftover removal is under way in or around
// the area.
func (m *Manager) claimedLocked(area string) bool {
	return slices.ContainsFunc(m.claims, func(claim string) bool { return overlaps(claim, area) })
}

// CleanupVerdict says whether the leftover at path may be removed now. A
// leftover whose download still has a record is never removable: the record
// is the proof the files are in use, whatever the list says.
//
//wails:ignore
func (m *Manager) CleanupVerdict(path string) (ok bool, code, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanupVerdictLocked(path)
}

func (m *Manager) cleanupVerdictLocked(path string) (ok bool, code, reason string) {
	idx := slices.IndexFunc(m.cleanups, func(c Cleanup) bool { return platform.SamePath(c.Path, path) })
	if idx < 0 {
		return false, diskreason.NoRecord, "нет записи об оставшихся файлах"
	}
	if id := m.cleanups[idx].DownloadID; id != "" && m.findLocked(id) != nil {
		return false, diskreason.OwnedByDownload, "запись загрузки ещё существует"
	}
	if m.coveredLocked(path) {
		return false, diskreason.OwnedByDownload, "файлы принадлежат текущей загрузке"
	}
	return true, "", ""
}

// ReserveCleanup checks the leftover and, in the same step, bars every new
// download from writing into or around it until release is called.
//
//wails:ignore
func (m *Manager) ReserveCleanup(path string) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ok, code, _ := m.cleanupVerdictLocked(path); !ok {
		if code == diskreason.NoRecord {
			return nil, errCleanupUnknown
		}
		return nil, errCleanupInUse
	}
	m.claims = append(m.claims, path)
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if i := slices.Index(m.claims, path); i >= 0 {
			m.claims = slices.Delete(m.claims, i, i+1)
		}
	}, nil
}

// RemoveCleanup deletes the leftover and its entry. Nothing here runs by
// itself: the user asks for it from the disk screen, after the owners of the
// surrounding folders have been asked.
//
//wails:ignore
func (m *Manager) RemoveCleanup(path string) error {
	release, err := m.ReserveCleanup(path)
	if err != nil {
		return err
	}
	defer release()
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return m.dropCleanup([]string{path})
	case err != nil:
		return fmt.Errorf("stat %s: %w", path, err)
	case info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0:
		return errCleanupUnsafe
	}
	if err := removeAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return m.dropCleanup([]string{path})
}
