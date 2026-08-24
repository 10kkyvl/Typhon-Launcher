package install

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

const (
	shortcutScanLimit  = 512 * 1024
	shortcutMaxEntries = 20000
)

var errShellTooLarge = errors.New("слишком много ярлыков для проверки")

type shellSnapshot struct {
	roots   []string
	entries map[string]bool
	taken   bool
}

func takeShellSnapshot(ctx context.Context, roots []string) (shellSnapshot, error) {
	snap := shellSnapshot{roots: roots, entries: make(map[string]bool), taken: true}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if err := scanShell(ctx, root, snap.entries); err != nil {
			return shellSnapshot{}, err
		}
	}
	return snap, nil
}

func scanShell(ctx context.Context, root string, out map[string]bool) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if len(out) >= shortcutMaxEntries {
			return errShellTooLarge
		}
		out[strings.ToLower(path)] = d.IsDir()
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// cleanShellShortcuts удаляет только то, что появилось за время установки и
// ссылается на её каталог: ярлык, созданный пользователем в тот же момент,
// проверку по цели не проходит и остаётся на месте.
func cleanShellShortcuts(ctx context.Context, before shellSnapshot, target string) ([]string, error) {
	if !before.taken || target == "" {
		return nil, nil
	}
	after, err := takeShellSnapshot(ctx, before.roots)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, 8)
	dirs := make([]string, 0, 4)
	for path, isDir := range after.entries {
		if _, existed := before.entries[path]; existed {
			continue
		}
		if isDir {
			dirs = append(dirs, path)
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))

	removed := make([]string, 0, len(files))
	var failures []error
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		match, err := shortcutPointsTo(path, target)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !match {
			continue
		}
		if err := os.Remove(path); err != nil {
			failures = append(failures, err)
			continue
		}
		removed = append(removed, path)
	}
	for _, path := range dirs {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		empty, err := dirEmpty(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !empty {
			continue
		}
		if err := os.Remove(path); err != nil {
			failures = append(failures, err)
			continue
		}
		removed = append(removed, path)
	}
	return removed, errors.Join(failures...)
}

func shortcutPointsTo(path, target string) (bool, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".lnk", ".url", ".pif":
	default:
		return false, nil
	}
	data, err := readHead(path, shortcutScanLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return referencesPath(data, target), nil
}

func readHead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close shortcut", "path", path, "error", err)
		}
	}()
	return io.ReadAll(io.LimitReader(f, limit))
}

// Цель ярлыка лежит в .lnk и как ANSI-строка, и как UTF-16LE, а .url хранит её
// в виде file:///C:/..., поэтому путь ищем во всех трёх видах.
func referencesPath(data []byte, target string) bool {
	if target == "" || len(data) == 0 {
		return false
	}
	clean := filepath.Clean(target)
	variants := []string{clean, strings.ReplaceAll(clean, `\`, "/")}
	lower := bytes.ToLower(data)
	for _, variant := range variants {
		needle := []byte(variant)
		if bytes.Contains(data, needle) || bytes.Contains(lower, bytes.ToLower(needle)) {
			return true
		}
		wide := utf16Bytes(variant)
		if bytes.Contains(data, wide) || bytes.Contains(lower, bytes.ToLower(wide)) {
			return true
		}
	}
	return false
}

func utf16Bytes(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[i*2:], u)
	}
	return out
}
