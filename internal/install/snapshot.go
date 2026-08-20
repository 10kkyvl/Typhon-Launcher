package install

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const snapshotDepth = 2

type dirState struct {
	modTime  time.Time
	children string
}

type fsSnapshot struct {
	roots []string
	dirs  map[string]dirState
}

func takeSnapshot(roots []string) fsSnapshot {
	snap := fsSnapshot{dirs: map[string]dirState{}}
	for _, root := range roots {
		root = filepath.Clean(root)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		snap.roots = append(snap.roots, root)
		scanDir(snap.dirs, root, snapshotDepth)
	}
	return snap
}

func scanDir(out map[string]dirState, dir string, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	subdirs := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		}
	}
	sort.Strings(names)

	state := dirState{children: strings.Join(names, "\n")}
	if info, err := os.Stat(dir); err == nil {
		state.modTime = info.ModTime()
	}
	out[dir] = state

	if depth <= 0 {
		return
	}
	for _, name := range subdirs {
		scanDir(out, filepath.Join(dir, name), depth-1)
	}
}

func diffSnapshot(before, after fsSnapshot) []string {
	skip := make(map[string]bool, len(after.roots))
	for _, root := range after.roots {
		skip[root] = true
	}

	changed := make([]string, 0, 8)
	for path, state := range after.dirs {
		if skip[path] {
			continue
		}
		old, known := before.dirs[path]
		if known && old.modTime.Equal(state.modTime) && old.children == state.children {
			continue
		}
		changed = append(changed, path)
	}
	return shallowest(changed)
}

func shallowest(paths []string) []string {
	sort.Slice(paths, func(i, j int) bool {
		a := strings.Count(paths[i], string(filepath.Separator))
		b := strings.Count(paths[j], string(filepath.Separator))
		if a == b {
			return paths[i] < paths[j]
		}
		return a < b
	})
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		nested := false
		for _, kept := range out {
			if inside(kept, path) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, path)
		}
	}
	return out
}
