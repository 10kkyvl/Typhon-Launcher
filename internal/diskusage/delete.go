package diskusage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"typhon/internal/download"
	"typhon/internal/platform"
)

var removeAll = os.RemoveAll

func (s *Service) leftoverOf(path string) (Leftover, entry, bool) {
	key, err := platform.PathKey(path)
	if err != nil {
		return Leftover{}, entry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return Leftover{}, entry{}, false
	}
	e, ok := s.entries[key]
	if !ok {
		return Leftover{}, entry{}, false
	}
	for _, l := range s.report.Leftovers {
		if platform.SamePath(l.Path, path) {
			return l, e, true
		}
	}
	return Leftover{}, entry{}, false
}

func (s *Service) dropLeftover(path string) {
	key, err := platform.PathKey(path)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	if s.report == nil {
		return
	}
	s.report.Leftovers = slices.DeleteFunc(s.report.Leftovers, func(l Leftover) bool {
		return platform.SamePath(l.Path, path)
	})
	s.report.recount()
}

// DeleteLeftover removes a leftover that the last scan listed. The scan's
// verdict is not trusted: the owner is asked again, the path must still be a
// plain folder or file directly inside the folder it was found in, and a link
// is never followed.
func (s *Service) DeleteLeftover(ctx context.Context, path string) error {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	found, owner, ok := s.leftoverOf(path)
	if !ok {
		return fmt.Errorf("%w: %s", errNotScanned, path)
	}
	if !found.Deletable {
		return fmt.Errorf("%w: %s", errNotDeletable, found.Reason)
	}
	// An owner that can reserve the path checks and holds off new jobs in one
	// step; the others are asked again right before the removal below.
	release := func() {}
	if owner.reserve != nil {
		held, err := owner.reserve()
		if err != nil {
			return fmt.Errorf("%w: %w", errNotDeletable, err)
		}
		release = held
	} else if ok, _, reason := owner.check(); !ok {
		return fmt.Errorf("%w: %s", errNotDeletable, reason)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(found.Path)
	if errors.Is(err, fs.ErrNotExist) {
		s.dropLeftover(found.Path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", found.Path, err)
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 ||
		!platform.Inside(owner.root, found.Path) || platform.SamePath(owner.root, found.Path) {
		return fmt.Errorf("%w: %s", errUnsafePath, found.Path)
	}
	if owner.reserve == nil {
		if ok, _, reason := owner.check(); !ok {
			return fmt.Errorf("%w: %s", errNotDeletable, reason)
		}
	}
	remove := func() error { return removeAll(found.Path) }
	if owner.remove != nil {
		remove = owner.remove
	}
	if err := remove(); err != nil {
		return fmt.Errorf("delete %s: %w", found.Path, err)
	}
	s.dropLeftover(found.Path)
	return nil
}

// DeleteDownloadData deletes the files of a finished download after asking
// the installs once more whether anything still reads from them.
func (s *Service) DeleteDownloadData(ctx context.Context, id string) error {
	_, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	d, err := s.deps.Downloads.Get(id)
	if err != nil {
		return err
	}
	if err := s.deps.Installs.CheckDownloadDeletable(d); err != nil {
		return err
	}
	delErr := s.deps.Downloads.DeleteData(id)
	if _, getErr := s.deps.Downloads.Get(id); errors.Is(getErr, download.ErrNotFound) {
		s.dropDownload(id)
	}
	return delErr
}

func (s *Service) dropDownload(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return
	}
	s.report.Downloads = slices.DeleteFunc(s.report.Downloads, func(d DownloadUsage) bool { return d.ID == id })
	s.report.recount()
}

// DeleteScreenshots deletes the screenshots of a game the last scan counted,
// one by one through the screenshot service, and says how many went.
func (s *Service) DeleteScreenshots(ctx context.Context, gameID string) (GroupDelete, error) {
	return s.deleteGroup(ctx, groupKey{groupShots, gameID}, s.deleteShot(gameID), s.listShots(gameID))
}

// DeleteSnapshots deletes the local save copies of a game the last scan
// counted and says how many went.
func (s *Service) DeleteSnapshots(ctx context.Context, gameID string) (GroupDelete, error) {
	return s.deleteGroup(ctx, groupKey{groupSnaps, gameID}, s.deleteSnap(gameID), s.listSnaps(gameID))
}
