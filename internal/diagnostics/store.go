package diagnostics

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"typhon/internal/storage"
)

const (
	pendingVersion  = 1
	maxPendingFiles = 20
)

func pendingDirFrom(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "diagnostics", "pending")
}

func pendingFilePath(dir string, now time.Time) (string, error) {
	ts := now.UnixNano()
	for {
		path := filepath.Join(dir, strconv.FormatInt(ts, 10)+".json")
		_, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		ts++
	}
}

func listPendingFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		return pendingTimestamp(names[i]) < pendingTimestamp(names[j])
	})
	return names, nil
}

func pendingTimestamp(name string) int64 {
	ts, err := strconv.ParseInt(strings.TrimSuffix(name, ".json"), 10, 64)
	if err != nil {
		return 0
	}
	return ts
}

func savePending(dir string, now time.Time, batch []reportPayload) error {
	if dir == "" {
		return errors.New("diagnostics pending dir unavailable")
	}
	names, err := listPendingFiles(dir)
	if err != nil {
		return fmt.Errorf("list pending diagnostics: %w", err)
	}
	for len(names) >= maxPendingFiles {
		oldest := filepath.Join(dir, names[0])
		if err := os.Remove(oldest); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("evict pending diagnostics: %w", err)
		}
		names = names[1:]
	}
	path, err := pendingFilePath(dir, now)
	if err != nil {
		return fmt.Errorf("allocate pending diagnostics path: %w", err)
	}
	return storage.Save(path, pendingVersion, batch)
}

func loadPending(path string) ([]reportPayload, error) {
	var batch []reportPayload
	if err := storage.Load(path, pendingVersion, nil, &batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func removePendingDir(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove pending diagnostics dir: %w", err)
	}
	return nil
}
