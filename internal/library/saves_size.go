package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxSavesBytes is the size past which a save folder is neither synced to the
// cloud nor worth backing up; cloudsaves takes its unpacked limit from here.
const MaxSavesBytes int64 = 1 << 30

const (
	verdictTTL        = time.Hour
	verdictCacheLimit = 256
)

var (
	maxHeuristicSavesBytes   = MaxSavesBytes
	maxHeuristicSavesEntries = 200000
	walkDir                  = filepath.WalkDir
	verdictNow               = time.Now
)

var cacheDirSuffixes = []string{"cache"}

var cacheDirNames = []string{"logs", "log", "crashes", "crashdumps", "dumps", "temp", "tmp"}

type heuristicVerdict struct {
	reason     string
	unreadable int
}

type verdictKey struct {
	mod     time.Time
	entries int
}

type cachedVerdict struct {
	key     verdictKey
	at      time.Time
	verdict heuristicVerdict
}

var (
	verdictMu    sync.Mutex
	verdictCache = map[string]cachedVerdict{}
)

// vetHeuristicSaves decides whether a folder found by name matching can be
// taken for saves. Folders from a manual choice or from the Ludusavi manifest
// never come here. The walk stops as soon as the size limit is crossed. At the
// entry limit it stops too, but the verdict is made from what was counted, so
// a big world is not rejected for its file count alone. Any walk failure other
// than a cancelled context accepts the folder, as before the check existed.
func vetHeuristicSaves(ctx context.Context, root string) (heuristicVerdict, bool, error) {
	if err := ctx.Err(); err != nil {
		return heuristicVerdict{}, false, err
	}
	key, keyed := folderKey(root)
	if keyed {
		if v, ok := lookupVerdict(root, key); ok {
			return v, true, nil
		}
	}
	verdict, complete, err := measureSaves(ctx, root)
	if err != nil {
		return heuristicVerdict{}, false, err
	}
	if keyed && complete {
		storeVerdict(root, key, verdict)
	}
	return verdict, false, nil
}

func measureSaves(ctx context.Context, root string) (heuristicVerdict, bool, error) {
	var total, cached int64
	var entries int
	var verdict heuristicVerdict
	errStop := errors.New("limit crossed")

	err := walkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			verdict.unreadable += unreadableCount(walkErr)
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
			verdict.unreadable += unreadableCount(err)
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
		if entries >= maxHeuristicSavesEntries {
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return heuristicVerdict{}, false, ctxErr
		}
		slog.Warn("save folder guess not measured, accepted unchecked", "path", root, "error", err)
		return heuristicVerdict{unreadable: verdict.unreadable + 1}, false, nil
	}
	if verdict.reason == "" && total > 0 && cached*2 > total {
		verdict.reason = "mostly cache, log and dump folders"
	}
	return verdict, true, nil
}

func unreadableCount(err error) int {
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	return 1
}

func folderKey(root string) (verdictKey, bool) {
	info, err := os.Stat(root)
	if err != nil {
		return verdictKey{}, false
	}
	names, err := os.ReadDir(root)
	if err != nil {
		return verdictKey{}, false
	}
	return verdictKey{mod: info.ModTime(), entries: len(names)}, true
}

func lookupVerdict(root string, key verdictKey) (heuristicVerdict, bool) {
	verdictMu.Lock()
	defer verdictMu.Unlock()
	c, ok := verdictCache[root]
	if !ok || c.key != key || verdictNow().Sub(c.at) > verdictTTL {
		return heuristicVerdict{}, false
	}
	return c.verdict, true
}

func storeVerdict(root string, key verdictKey, v heuristicVerdict) {
	verdictMu.Lock()
	defer verdictMu.Unlock()
	if _, ok := verdictCache[root]; !ok && len(verdictCache) >= verdictCacheLimit {
		clear(verdictCache)
	}
	verdictCache[root] = cachedVerdict{key: key, at: verdictNow(), verdict: v}
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
