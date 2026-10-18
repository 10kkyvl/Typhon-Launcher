package procs

import "slices"

// MaxAncestorDepth bounds how far up the parent chain a lookup climbs: a
// real launcher-wrapper chain is a few levels deep, and the pid reuse that
// can form a loop in a snapshot must not turn into an endless walk.
const MaxAncestorDepth = 16

// Ancestors lists the parents of pid from the nearest outwards, taken from a
// pid-to-parent snapshot. It stops at a missing or zero parent, at a pid it
// has already seen, and after MaxAncestorDepth steps.
func Ancestors(parents map[uint32]uint32, pid uint32) []uint32 {
	var out []uint32
	seen := map[uint32]struct{}{pid: {}}
	cur := pid
	for len(out) < MaxAncestorDepth {
		parent, ok := parents[cur]
		if !ok || parent == 0 {
			break
		}
		if _, dup := seen[parent]; dup {
			break
		}
		seen[parent] = struct{}{}
		out = append(out, parent)
		cur = parent
	}
	return out
}

// Descendants lists every process below root in a pid-to-parent snapshot,
// deepest first, so a caller that ends them in order never orphans a process
// it has yet to reach. root itself is not included.
func Descendants(parents map[uint32]uint32, root uint32) []uint32 {
	children := make(map[uint32][]uint32, len(parents))
	for pid, parent := range parents {
		if pid == parent || pid == 0 {
			continue
		}
		children[parent] = append(children[parent], pid)
	}
	seen := map[uint32]struct{}{root: {}}
	level := []uint32{root}
	var levels [][]uint32
	for len(level) > 0 {
		var next []uint32
		for _, pid := range level {
			for _, child := range children[pid] {
				if _, dup := seen[child]; dup {
					continue
				}
				seen[child] = struct{}{}
				next = append(next, child)
			}
		}
		if len(next) > 0 {
			slices.Sort(next)
			levels = append(levels, next)
		}
		level = next
	}
	var out []uint32
	for i := len(levels) - 1; i >= 0; i-- {
		out = append(out, levels[i]...)
	}
	return out
}
