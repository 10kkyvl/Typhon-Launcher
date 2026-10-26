package savebackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"typhon/internal/storage"
)

const (
	statFile    = "stat.json"
	maxStatSize = 64 << 20
)

// fileStat is what a cheap "nothing changed" check compares: no content, only
// the size and modification time the file had before the snapshot hashed it.
type fileStat struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

type statDoc struct {
	Source string     `json:"source"`
	Files  []fileStat `json:"files"`
}

func statTree(ctx context.Context, root string) ([]fileStat, error) {
	var out []fileStat
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, fileStat{Path: filepath.ToSlash(rel), Size: info.Size(), MTime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b fileStat) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// writeStat skips a record too large to read back: without one the next
// launch simply hashes, the same quiet fallback as for a snapshot made before
// records existed.
func writeStat(dir, source string, files []fileStat) error {
	data, err := json.Marshal(statDoc{Source: source, Files: files})
	if err != nil {
		return err
	}
	if len(data) > maxStatSize {
		return nil
	}
	return storage.WriteAtomic(filepath.Join(dir, statFile), data)
}

// unchangedSince reports whether the snapshot recorded the same files, sizes
// and times as now. A snapshot without a usable record (made by an older
// version, or unreadable) is not a match: the caller falls back to hashing.
func unchangedSince(snap Snapshot, source string, now []fileStat) bool {
	if snap.Broken || snap.Path == "" {
		return false
	}
	data, err := readLimited(filepath.Join(filepath.Dir(snap.Path), statFile), maxStatSize)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("snapshot stat record unusable, hashing instead", "snapshot", snap.ID, "error", err)
		}
		return false
	}
	var doc statDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		slog.Warn("snapshot stat record unusable, hashing instead", "snapshot", snap.ID, "error", fmt.Errorf("stat.json: %w", err))
		return false
	}
	return doc.Source == source && slices.Equal(doc.Files, now)
}
