package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const snapshotDepth = 2

type dirState struct {
	modTime    time.Time
	children   string
	unreadable bool
}

type fsSnapshot struct {
	roots []string
	dirs  map[string]dirState
}

func takeSnapshot(roots []string) (fsSnapshot, error) {
	snap := fsSnapshot{dirs: map[string]dirState{}}
	for _, root := range roots {
		root = filepath.Clean(root)
		info, err := os.Stat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fsSnapshot{}, fmt.Errorf("stat %s: %w", root, err)
		}
		if !info.IsDir() {
			continue
		}
		snap.roots = append(snap.roots, root)
		if err := scanDir(snap.dirs, root, snapshotDepth); err != nil {
			return fsSnapshot{}, err
		}
	}
	return snap, nil
}

func scanDir(out map[string]dirState, dir string, depth int) error {
	entries, err := os.ReadDir(dir)
	// Каталоги вида C:\Program Files\WindowsApps закрыты ACL и читаться не будут
	// никогда; отмечаем их как непрочитанные, чтобы отличать от «каталог пуст».
	if errors.Is(err, fs.ErrPermission) {
		out[dir] = dirState{unreadable: true}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
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

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	out[dir] = dirState{children: strings.Join(names, "\n"), modTime: info.ModTime()}

	if depth <= 0 {
		return nil
	}
	for _, name := range subdirs {
		if err := scanDir(out, filepath.Join(dir, name), depth-1); err != nil {
			return err
		}
	}
	return nil
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
		if known && old.unreadable == state.unreadable && old.modTime.Equal(state.modTime) && old.children == state.children {
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
