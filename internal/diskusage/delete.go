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

// DeleteScreenshots deletes every screenshot of a game one by one through the
// screenshot service and reports what could not be deleted.
func (s *Service) DeleteScreenshots(ctx context.Context, gameID string) error {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if gameID == "" {
		return errEmptyGameID
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()

	shots, err := s.deps.Screenshots.List(gameID)
	if err != nil {
		return err
	}
	var errs []error
	for _, shot := range shots {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := s.deps.Screenshots.Delete(gameID, shot.Name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	left, listErr := s.deps.Screenshots.List(gameID)
	if listErr != nil {
		errs = append(errs, listErr)
		return errors.Join(errs...)
	}
	var size int64
	for _, shot := range left {
		size += shot.Size
	}
	s.setGroup(gameID, true, len(left), size)
	return errors.Join(errs...)
}

// DeleteSnapshots deletes every local save snapshot of a game.
func (s *Service) DeleteSnapshots(ctx context.Context, gameID string) error {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if gameID == "" {
		return errEmptyGameID
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()

	snaps, err := s.deps.Snapshots.List(ctx, gameID)
	if err != nil {
		return err
	}
	var errs []error
	for _, snap := range snaps {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := s.deps.Snapshots.Delete(gameID, snap.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", snap.ID, err))
		}
	}
	left, listErr := s.deps.Snapshots.List(ctx, gameID)
	if listErr != nil {
		errs = append(errs, listErr)
		return errors.Join(errs...)
	}
	var size int64
	unknown := false
	for _, snap := range left {
		size += snap.SizeBytes
		unknown = unknown || snap.Broken
	}
	s.setGroup(gameID, false, len(left), size)
	if unknown {
		s.markGroupUnknown(gameID)
	}
	return errors.Join(errs...)
}

func (s *Service) setGroup(gameID string, shots bool, count int, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return
	}
	groups := &s.report.Snapshots
	if shots {
		groups = &s.report.Screenshots
	}
	if count == 0 {
		*groups = slices.DeleteFunc(*groups, func(g GroupUsage) bool { return g.GameID == gameID })
	} else {
		for i := range *groups {
			if (*groups)[i].GameID == gameID {
				(*groups)[i].Count, (*groups)[i].SizeBytes, (*groups)[i].SizeUnknown = count, size, false
			}
		}
	}
	s.report.recount()
}

func (s *Service) markGroupUnknown(gameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return
	}
	for i := range s.report.Snapshots {
		if s.report.Snapshots[i].GameID == gameID {
			s.report.Snapshots[i].SizeUnknown = true
		}
	}
	s.report.recount()
}
