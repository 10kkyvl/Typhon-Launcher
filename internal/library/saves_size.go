package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

var (
	maxHeuristicSavesBytes   int64 = 256 << 20
	maxHeuristicSavesEntries       = 20000
)

var cacheDirSuffixes = []string{"cache"}

var cacheDirNames = []string{"logs", "log", "crashes", "crashdumps", "dumps", "temp", "tmp"}

var rejectedSavesLogged sync.Map

type heuristicVerdict struct {
	reason     string
	unreadable int
}

// vetHeuristicSaves decides whether a folder found by name matching can be
// taken for saves. Folders from a manual choice or from the Ludusavi manifest
// never come here. The walk stops as soon as a limit is crossed, so a huge
// client cache costs a bounded amount of work.
func vetHeuristicSaves(ctx context.Context, root string) (heuristicVerdict, error) {
	var total, cached int64
	var entries int
	var verdict heuristicVerdict
	errStop := errors.New("limit crossed")

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			unreadable, fatal := classifyDirErr(walkErr)
			if fatal {
				return walkErr
			}
			verdict.unreadable += unreadable
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			unreadable, fatal := classifyDirErr(err)
			if fatal {
				return err
			}
			verdict.unreadable += unreadable
			return nil
		}
		entries++
		total += info.Size()
		if inCacheDir(root, path) {
			cached += info.Size()
		}
		if total > maxHeuristicSavesBytes {
			verdict.reason = fmt.Sprintf("larger than %d MB", maxHeuristicSavesBytes>>20)
			return errStop
		}
		if entries > maxHeuristicSavesEntries {
			verdict.reason = fmt.Sprintf("more than %d files", maxHeuristicSavesEntries)
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return heuristicVerdict{}, fmt.Errorf("measure %s: %w", root, err)
	}
	if verdict.reason == "" && total > 0 && cached*2 > total {
		verdict.reason = "mostly cache, log and dump folders"
	}
	return verdict, nil
}

func inCacheDir(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if isCacheDirName(part) {
			return true
		}
	}
	return false
}

func isCacheDirName(name string) bool {
	lower := strings.ToLower(name)
	for _, n := range cacheDirNames {
		if lower == n {
			return true
		}
	}
	for _, s := range cacheDirSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

func logRejectedSaves(path, reason string) {
	if _, seen := rejectedSavesLogged.LoadOrStore(path+"|"+reason, true); seen {
		return
	}
	slog.Info("save folder guess rejected", "path", path, "reason", reason)
}
