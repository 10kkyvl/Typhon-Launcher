package install

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	shortcutScanLimit  = 512 * 1024
	shortcutMaxEntries = 20000
)

var (
	errShellTooLarge = errors.New("слишком много ярлыков для проверки")
	errBadShellLink  = errors.New("повреждённый ярлык")
)

type shellEntry struct {
	dir  bool
	size int64
	mod  time.Time
	path string
}

// rootIDs — каким был каждый корень в момент снимка: удаление сверяет с ним
// записи, а не разбирает корень заново, поэтому корень, подменённый ссылкой
// после снимка, не уводит удаление в другой каталог.
type shellSnapshot struct {
	roots   []string
	entries map[string]shellEntry
	rootIDs map[string]shellRootID
	taken   bool
}

func takeShellSnapshot(ctx context.Context, roots []string) (shellSnapshot, error) {
	snap := shellSnapshot{roots: roots, entries: make(map[string]shellEntry), rootIDs: make(map[string]shellRootID, len(roots)), taken: true}
	for _, root := range roots {
		if root == "" {
			continue
		}
		// Корень опознаётся до обхода: подменённый во время обхода, он уже не
		// совпадёт с итоговыми путями найденных записей.
		id, err := identifyShellRoot(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return shellSnapshot{}, err
		}
		snap.rootIDs[root] = id
		if err := scanShell(ctx, root, snap.entries); err != nil {
			return shellSnapshot{}, err
		}
	}
	return snap, nil
}

func scanShell(ctx context.Context, root string, out map[string]shellEntry) error {
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
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		// Ключ в нижнем регистре нужен для сравнения на Windows, а файловые
		// операции идут по настоящему пути: на регистрозависимой ФС ключ не открывается.
		out[strings.ToLower(path)] = shellEntry{dir: d.IsDir(), size: info.Size(), mod: info.ModTime(), path: path}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// cleanShellShortcuts удаляет только то, что установщик создал или переписал:
// ярлык, созданный пользователем в тот же момент, проверку по цели не проходит
// и остаётся на месте. Ярлык игры (ссылается на её каталог) убирается только
// при game и только новый. Ярлык сайта убирается всегда, в том числе
// перезаписанный: репаки Игрухи каждый раз переписывают один и тот же ярлык на
// общем рабочем столе. Но только прямо в корне или в новой папке установщика —
// в своих подпапках пользователь держит закладки, и их это не касается.
func cleanShellShortcuts(ctx context.Context, before shellSnapshot, target string, game bool) ([]string, error) {
	if !before.taken {
		return nil, nil
	}
	after, err := takeShellSnapshot(ctx, before.roots)
	if err != nil {
		return nil, err
	}
	return removeShellShortcuts(ctx, before, after, target, game)
}

func removeShellShortcuts(ctx context.Context, before, after shellSnapshot, target string, game bool) ([]string, error) {
	installerDirs := make(map[string]bool, len(before.roots))
	for _, root := range before.roots {
		if root != "" {
			installerDirs[strings.ToLower(filepath.Clean(root))] = true
		}
	}
	files := make([]string, 0, 8)
	rewritten := make([]string, 0, 4)
	dirs := make([]string, 0, 4)
	for path, entry := range after.entries {
		old, existed := before.entries[path]
		switch {
		case existed && !entry.dir && !old.dir && (old.size != entry.size || !old.mod.Equal(entry.mod)):
			rewritten = append(rewritten, path)
		case existed:
		case entry.dir:
			dirs = append(dirs, path)
			installerDirs[path] = true
		default:
			files = append(files, path)
		}
	}
	sort.Strings(files)
	sort.Strings(rewritten)
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))

	removed := make([]string, 0, len(files)+len(rewritten))
	emptied := make(map[string]bool, 4)
	var failures []error
	drop := func(path string, fresh bool) (err error) {
		onDisk := after.entries[path].path
		ext := strings.ToLower(filepath.Ext(onDisk))
		gameRule := fresh && game && target != "" && gameShortcutExts[ext]
		siteRule := installerDirs[filepath.Dir(path)] && siteShortcutExts[ext]
		if !gameRule && !siteRule {
			return nil
		}
		entry, err := openShellEntry(after, onDisk, false)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, entry.close()) }()
		data, err := entry.read(shortcutScanLimit)
		if err != nil {
			return err
		}
		match := gameRule && referencesPath(data, target)
		if !match && siteRule {
			if match, err = siteShortcut(onDisk, data); err != nil {
				return err
			}
		}
		if !match {
			return nil
		}
		if err := removeShellEntry(entry); err != nil {
			return err
		}
		removed = append(removed, path)
		emptied[filepath.Dir(path)] = true
		return nil
	}
	dropDir := func(path string) (gone bool, err error) {
		entry, err := openShellEntry(after, after.entries[path].path, true)
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		defer func() { err = errors.Join(err, entry.close()) }()
		err = removeShellEntry(entry)
		if errors.Is(err, errShellDirNotEmpty) {
			return false, nil
		}
		return err == nil, err
	}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if err := drop(path, true); err != nil {
			failures = append(failures, err)
		}
	}
	for _, path := range rewritten {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if err := drop(path, false); err != nil {
			failures = append(failures, err)
		}
	}
	for _, path := range dirs {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if !game && !emptied[path] {
			continue
		}
		gone, err := dropDir(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !gone {
			continue
		}
		removed = append(removed, path)
		emptied[filepath.Dir(path)] = true
	}
	return removed, errors.Join(failures...)
}

var (
	gameShortcutExts = map[string]bool{".lnk": true, ".url": true, ".pif": true}
	siteShortcutExts = map[string]bool{".lnk": true, ".url": true}
)

// siteShortcut узнаёт ярлык сайта: .url с адресом http(s) или .lnk, цель
// которого — .url-файл. Так делают репаки Игрухи: ярлык на общем столе ведёт в
// Program Files (x86)\TI\TI.URL, а тот — на сайт.
func siteShortcut(path string, data []byte) (bool, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".url":
		return webURL(data), nil
	case ".lnk":
		link, err := lnkTarget(data)
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
		return strings.EqualFold(filepath.Ext(link), ".url"), nil
	default:
		return false, nil
	}
}

func webURL(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(line)), "url=")
		if ok && (strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")) {
			return true
		}
	}
	return false
}

const (
	lnkHeaderSize      = 0x4c
	lnkHasIDList       = 0x1
	lnkHasLinkInfo     = 0x2
	lnkHasName         = 0x4
	lnkHasRelativePath = 0x8
	lnkIsUnicode       = 0x80
	lnkLocalBasePath   = 0x1
)

// lnkTarget читает путь цели из .lnk по MS-SHLLINK: из LinkInfo, а без него —
// из RELATIVE_PATH. Ярлык на объект оболочки без пути даёт "".
func lnkTarget(data []byte) (string, error) {
	if len(data) < lnkHeaderSize || binary.LittleEndian.Uint32(data) != lnkHeaderSize {
		return "", errBadShellLink
	}
	flags := binary.LittleEndian.Uint32(data[0x14:])
	offset := lnkHeaderSize
	if flags&lnkHasIDList != 0 {
		if len(data) < offset+2 {
			return "", errBadShellLink
		}
		offset += 2 + int(binary.LittleEndian.Uint16(data[offset:]))
	}
	if flags&lnkHasLinkInfo != 0 {
		if len(data) < offset+0x1c {
			return "", errBadShellLink
		}
		size := int(binary.LittleEndian.Uint32(data[offset:]))
		if size < 0x1c || size > len(data)-offset {
			return "", errBadShellLink
		}
		info := data[offset : offset+size]
		if binary.LittleEndian.Uint32(info[8:])&lnkLocalBasePath != 0 {
			return lnkInfoPath(info)
		}
		offset += size
	}
	width := 1
	if flags&lnkIsUnicode != 0 {
		width = 2
	}
	for _, bit := range []uint32{lnkHasName, lnkHasRelativePath} {
		if flags&bit == 0 {
			continue
		}
		if len(data) < offset+2 {
			return "", errBadShellLink
		}
		count := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2
		if len(data) < offset+count*width {
			return "", errBadShellLink
		}
		raw := data[offset : offset+count*width]
		offset += count * width
		if bit != lnkHasRelativePath {
			continue
		}
		if width == 1 {
			return string(raw), nil
		}
		units := make([]uint16, count)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(raw[i*2:])
		}
		return string(utf16.Decode(units)), nil
	}
	return "", nil
}

// ANSI-пути в LinkInfo записаны в кодовой странице системы; для проверки
// расширения этого хватает, но широкие варианты точнее, поэтому они первые.
func lnkInfoPath(info []byte) (string, error) {
	field := func(at int) uint32 { return binary.LittleEndian.Uint32(info[at:]) }
	if field(4) >= 0x24 && len(info) >= 0x24 {
		base, err := lnkString(info, field(0x1c), true)
		if err != nil {
			return "", err
		}
		suffix, err := lnkString(info, field(0x20), true)
		if err != nil {
			return "", err
		}
		if base != "" {
			return base + suffix, nil
		}
	}
	base, err := lnkString(info, field(0x10), false)
	if err != nil {
		return "", err
	}
	suffix, err := lnkString(info, field(0x18), false)
	if err != nil {
		return "", err
	}
	return base + suffix, nil
}

func lnkString(info []byte, at uint32, wide bool) (string, error) {
	if at == 0 {
		return "", nil
	}
	if uint64(at) >= uint64(len(info)) {
		return "", errBadShellLink
	}
	rest := info[at:]
	if !wide {
		end := bytes.IndexByte(rest, 0)
		if end < 0 {
			return "", errBadShellLink
		}
		return string(rest[:end]), nil
	}
	units := make([]uint16, 0, 64)
	for i := 0; i+2 <= len(rest); i += 2 {
		u := binary.LittleEndian.Uint16(rest[i:])
		if u == 0 {
			return string(utf16.Decode(units)), nil
		}
		units = append(units, u)
	}
	return "", errBadShellLink
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
