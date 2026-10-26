package crashlog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

type DumpDir struct {
	Path     string
	MatchExe bool
	MaxDepth int
}

var errFoundDump = errors.New("dump found")

// FindDump reports whether a *.dmp written during [from, to] exists. A
// missing directory is not an error: most machines have no CrashDumps folder.
func FindDump(ctx context.Context, dirs []DumpDir, exe string, from, to time.Time) (bool, error) {
	var errs []error
	for _, d := range dirs {
		if d.Path == "" {
			continue
		}
		found, err := scanDumps(ctx, d, exe, from, to)
		if found {
			return true, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, err
			}
			errs = append(errs, err)
		}
	}
	return false, errors.Join(errs...)
}

func scanDumps(ctx context.Context, d DumpDir, exe string, from, to time.Time) (bool, error) {
	var errs []error
	root := filepath.Clean(d.Path)
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			errs = append(errs, err)
			if e != nil && e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			if rel != "." && strings.Count(rel, string(filepath.Separator))+1 > d.MaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".dmp") {
			return nil
		}
		if d.MatchExe && !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(exe)) {
			return nil
		}
		info, ierr := e.Info()
		if ierr != nil {
			errs = append(errs, ierr)
			return nil
		}
		if mt := info.ModTime(); !mt.Before(from) && !mt.After(to) {
			return errFoundDump
		}
		return nil
	})
	switch {
	case errors.Is(err, errFoundDump):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("scan dumps in %s: %w", root, err)
	}
	return false, errors.Join(errs...)
}
