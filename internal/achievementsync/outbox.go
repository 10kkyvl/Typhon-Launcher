package achievementsync

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"typhon/internal/storage"
)

// outbox is not safe for concurrent use: the service holds its mutex around
// every call.
type outbox struct {
	path  string
	write func(path string, version int, data any) error
	items []entry
	// broken is set when the file exists but cannot be read. Every write is
	// then refused, so the unreadable file is never replaced by an empty one.
	broken error
}

func openOutbox(dir string, write func(path string, version int, data any) error) *outbox {
	ob := &outbox{path: filepath.Join(dir, storeFile), write: write}
	var loaded state
	err := storage.Load(ob.path, storeVersion, nil, &loaded)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		ob.broken = fmt.Errorf("%w: %w", ErrOutboxBroken, err)
	default:
		for _, it := range loaded.Entries {
			if it.Owner == "" {
				slog.Warn("achievements outbox entry without an owner dropped", "igdb", it.IGDBID, "key", it.Key)
				continue
			}
			ob.items = append(ob.items, it)
		}
	}
	return ob
}

// replace stores next in memory and on disk as one operation: when the write
// fails the memory stays what the disk holds.
func (ob *outbox) replace(next []entry) error {
	if ob.broken != nil {
		return ob.broken
	}
	if err := ob.write(ob.path, storeVersion, state{Entries: next}); err != nil {
		return fmt.Errorf("save achievements outbox: %w", err)
	}
	ob.items = next
	return nil
}

func fresh(e entry, now time.Time) bool {
	return now.Sub(e.UnlockedAt) <= maxAge
}

// add merges entries into the queue, drops the expired and keeps the newest
// maxEntries.
func (ob *outbox) add(add []entry, now time.Time) error {
	next := slices.Clone(ob.items)
	seen := make(map[identity]struct{}, len(next))
	for _, e := range next {
		seen[e.id()] = struct{}{}
	}
	for _, e := range add {
		if _, ok := seen[e.id()]; ok {
			continue
		}
		seen[e.id()] = struct{}{}
		next = append(next, e)
	}
	next = pruned(next, now)
	return ob.replace(next)
}

func pruned(items []entry, now time.Time) []entry {
	out := items[:0:0]
	for _, e := range items {
		if fresh(e, now) {
			out = append(out, e)
		}
	}
	if over := len(out) - maxEntries; over > 0 {
		slices.SortStableFunc(out, func(a, b entry) int { return a.UnlockedAt.Compare(b.UnlockedAt) })
		slog.Warn("achievements outbox is full, oldest unlocks dropped", "dropped", over)
		out = out[over:]
	}
	return out
}

// settle removes the given entries and the expired ones.
func (ob *outbox) settle(done []entry, now time.Time) error {
	gone := make(map[identity]struct{}, len(done))
	for _, e := range done {
		gone[e.id()] = struct{}{}
	}
	var next []entry
	for _, e := range ob.items {
		if _, ok := gone[e.id()]; ok || !fresh(e, now) {
			continue
		}
		next = append(next, e)
	}
	if len(next) == len(ob.items) {
		return nil
	}
	return ob.replace(next)
}

// rebind hands the entries queued under a provisional owner to the account.
func (ob *outbox) rebind(from, to string, now time.Time) error {
	if from == to || !slices.ContainsFunc(ob.items, func(e entry) bool { return e.Owner == from }) {
		return nil
	}
	moved := slices.Clone(ob.items)
	for i := range moved {
		if moved[i].Owner == from {
			moved[i].Owner = to
		}
	}
	seen := make(map[identity]struct{}, len(moved))
	next := moved[:0:0]
	for _, e := range moved {
		if _, dup := seen[e.id()]; dup {
			continue
		}
		seen[e.id()] = struct{}{}
		next = append(next, e)
	}
	return ob.replace(pruned(next, now))
}
