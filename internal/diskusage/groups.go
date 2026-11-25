package diskusage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"

	"typhon/internal/savebackup"
	"typhon/internal/screenshots"
	"typhon/internal/uierr"
)

type groupKind int

const (
	groupShots groupKind = iota
	groupSnaps
)

func (k groupKind) String() string {
	if k == groupShots {
		return "screenshots"
	}
	return "snapshots"
}

type groupKey struct {
	kind   groupKind
	gameID string
}

// groupItem is one screenshot or save copy as a listing saw it.
type groupItem struct {
	id      string
	size    int64
	unknown bool
}

func shotItems(shots []screenshots.Shot) []groupItem {
	out := make([]groupItem, 0, len(shots))
	for _, s := range shots {
		out = append(out, groupItem{id: s.Name, size: s.Size})
	}
	return out
}

func snapItems(snaps []savebackup.Snapshot) []groupItem {
	out := make([]groupItem, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, groupItem{id: s.ID, size: s.SizeBytes, unknown: s.Broken})
	}
	return out
}

func tally(items []groupItem) (count int, size int64, unknown bool) {
	for _, it := range items {
		size += it.size
		unknown = unknown || it.unknown
	}
	return len(items), size, unknown
}

func itemIDs(items []groupItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.id)
	}
	return out
}

// addGroup lists a group from the items the scan listed one by one: what the
// report counts is exactly the set a later "delete all" works on. A group
// whose items could not be listed is shown from the usage numbers alone and
// has no set, so it cannot be deleted until a scan lists it.
func (sc *scanner) addGroup(key groupKey, items []groupItem, listErr error, fallback GroupUsage) {
	groups := &sc.report.Screenshots
	if key.kind == groupSnaps {
		groups = &sc.report.Snapshots
	}
	if listErr != nil {
		sc.warn(key.kind.String(), fmt.Errorf("%s: %w", key.gameID, listErr))
		*groups = append(*groups, sc.group(key.gameID, fallback.Count, fallback.SizeBytes, fallback.SizeUnknown))
		return
	}
	if len(items) == 0 {
		return
	}
	count, size, unknown := tally(items)
	*groups = append(*groups, sc.group(key.gameID, count, size, unknown))
	sc.sets[key] = itemIDs(items)
}

// groupSet is the set of items the report shows for a group.
func (s *Service) groupSet(key groupKey) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil || !slices.ContainsFunc(s.groupsLocked(key.kind), func(g GroupUsage) bool { return g.GameID == key.gameID }) {
		return nil, fmt.Errorf("%w: %s %s", errNotScanned, key.kind, key.gameID)
	}
	set, ok := s.sets[key]
	if !ok {
		return nil, fmt.Errorf("%w: the last scan could not list the %s of %s", errNotDeletable, key.kind, key.gameID)
	}
	return slices.Clone(set), nil
}

func (s *Service) groupsLocked(kind groupKind) []GroupUsage {
	if kind == groupShots {
		return s.report.Screenshots
	}
	return s.report.Snapshots
}

type answer int

const (
	answerDeleted answer = iota
	answerGone
	answerFailed
)

// deleteGroup deletes the items of a group the last report counted, and only
// them: a screenshot taken or a save copy written since stays. An item gone
// by then is not a failure. What is still on disk afterwards is listed again
// and becomes the group in the report, so the report and its set keep
// showing the same items.
func (s *Service) deleteGroup(ctx context.Context, key groupKey, del func(id string) (gone bool, err error), list func(context.Context) ([]groupItem, error)) (GroupDelete, error) {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return GroupDelete{}, err
	}
	defer end()
	if key.gameID == "" {
		return GroupDelete{}, errEmptyGameID
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()

	set, err := s.groupSet(key)
	if err != nil {
		return GroupDelete{}, err
	}
	s.stopWalk(errWalkStale)
	answers := make(map[string]answer, len(set))
	for i, id := range set {
		if ctx.Err() != nil {
			for _, rest := range set[i:] {
				answers[rest] = answerFailed
			}
			break
		}
		gone, err := del(id)
		switch {
		case err == nil:
			answers[id] = answerDeleted
		case gone:
			answers[id] = answerGone
		default:
			answers[id] = answerFailed
			slog.Warn("delete from disk usage", "group", key.kind.String(), "game", key.gameID, "item", id, "error", err)
		}
	}

	// The listing after the deletes says what is really left, also when the
	// deletes were cut short by a cancelled request.
	left, listErr := list(context.WithoutCancel(ctx))
	present := map[string]bool{}
	if listErr != nil {
		slog.Warn("list what a group delete left", "group", key.kind.String(), "game", key.gameID, "error", listErr)
	} else {
		for _, it := range left {
			present[it.id] = true
		}
	}
	var out GroupDelete
	for _, id := range set {
		a := answers[id]
		switch {
		case present[id]:
			a = answerFailed
		case listErr == nil && a == answerFailed:
			a = answerGone
		}
		switch a {
		case answerDeleted:
			out.Deleted++
		case answerGone:
			out.Gone++
		case answerFailed:
			out.Failed++
		}
	}
	if out.Failed > 0 {
		out.Code = DeletePartial
	}
	s.settleGroup(key, left, listErr == nil)
	return out, nil
}

// settleGroup puts what a delete left of a group into the report. When that
// could not be listed, the group's size is unknown and it has no set until
// the next scan.
func (s *Service) settleGroup(key groupKey, left []groupItem, known bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return
	}
	groups := &s.report.Screenshots
	if key.kind == groupSnaps {
		groups = &s.report.Snapshots
	}
	switch {
	case !known:
		delete(s.sets, key)
		for i := range *groups {
			if (*groups)[i].GameID == key.gameID {
				(*groups)[i].SizeUnknown = true
			}
		}
	case len(left) == 0:
		delete(s.sets, key)
		*groups = slices.DeleteFunc(*groups, func(g GroupUsage) bool { return g.GameID == key.gameID })
	default:
		s.sets[key] = itemIDs(left)
		count, size, unknown := tally(left)
		for i := range *groups {
			if (*groups)[i].GameID == key.gameID {
				(*groups)[i].Count, (*groups)[i].SizeBytes, (*groups)[i].SizeUnknown = count, size, unknown
			}
		}
	}
	s.report.recount()
}

func (s *Service) deleteShot(gameID string) func(string) (bool, error) {
	return func(name string) (bool, error) {
		err := s.deps.Screenshots.Delete(gameID, name)
		return errors.Is(err, fs.ErrNotExist), err
	}
}

func (s *Service) deleteSnap(gameID string) func(string) (bool, error) {
	return func(id string) (bool, error) {
		err := s.deps.Snapshots.Delete(gameID, id)
		return uierr.Code(err) == snapshotNotFound, err
	}
}

// snapshotNotFound is savebackup's answer for a copy that is not there, or
// whose record cannot be read: the listing after the deletes tells the two
// apart.
const snapshotNotFound = "savebackup.snapshot_not_found"

func (s *Service) listShots(gameID string) func(context.Context) ([]groupItem, error) {
	return func(context.Context) ([]groupItem, error) {
		shots, err := s.deps.Screenshots.List(gameID)
		if err != nil {
			return nil, err
		}
		return shotItems(shots), nil
	}
}

func (s *Service) listSnaps(gameID string) func(context.Context) ([]groupItem, error) {
	return func(ctx context.Context) ([]groupItem, error) {
		snaps, err := s.deps.Snapshots.List(ctx, gameID)
		if err != nil {
			return nil, err
		}
		return snapItems(snaps), nil
	}
}
