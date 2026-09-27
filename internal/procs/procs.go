// Package procs enumerates operating system processes with their full
// image path and start time. It has no notion of games, the library, or
// any other higher-level Typhon concept — it sits below all of that.
package procs

import (
	"sync"
	"time"
)

// Process is one entry read from the OS process table.
type Process struct {
	PID  uint32
	Path string // full image path; empty when PathUnknown
	// PathUnknown is set when the image path could not be read: no
	// permission, anti-cheat protection, or the process exited mid-scan.
	PathUnknown bool
	CreatedAt   time.Time
	// CreatedAtUnknown is set when the OS start time could not be read.
	CreatedAtUnknown bool
}

// imageCache remembers a process's resolved image path across enumeration
// ticks, keyed by pid together with the process's own creation time. The
// creation time is part of the key rather than a side check: once the OS
// recycles a pid, the new process's creation time differs, so a stale
// lookup misses on its own instead of needing separate invalidation logic.
type imageCache struct {
	mu      sync.Mutex
	entries map[uint32]imageCacheEntry
}

type imageCacheEntry struct {
	createdAt time.Time
	path      string
}

func newImageCache() *imageCache {
	return &imageCache{entries: make(map[uint32]imageCacheEntry)}
}

// lookup reports the cached path for pid, valid only if createdAt still
// matches what was cached — otherwise the pid has been recycled and the
// caller must resolve it again.
func (c *imageCache) lookup(pid uint32, createdAt time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[pid]
	if !ok || !e.createdAt.Equal(createdAt) {
		return "", false
	}
	return e.path, true
}

// store records the resolved path for pid at the given creation time,
// overwriting whatever was cached for that pid before.
func (c *imageCache) store(pid uint32, createdAt time.Time, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[pid] = imageCacheEntry{createdAt: createdAt, path: path}
}

// prune drops every cached pid not present in live, so a process that has
// exited is not remembered forever.
func (c *imageCache) prune(live map[uint32]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for pid := range c.entries {
		if _, ok := live[pid]; !ok {
			delete(c.entries, pid)
		}
	}
}
