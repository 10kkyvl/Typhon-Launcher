package install

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type dirLimits struct {
	closed []string
	roots  []string
}

func (l dirLimits) allows(dir string) bool {
	for _, closed := range l.closed {
		if inside(closed, dir) || inside(dir, closed) {
			return false
		}
	}
	for _, root := range l.roots {
		if inside(dir, root) {
			return false
		}
	}
	return true
}

func (s *Service) dirLimits(roots []string) dirLimits {
	closed := []string{s.config().DownloadsPath, s.store.dir, os.Getenv("SystemRoot")}
	if exe, err := os.Executable(); err != nil {
		slog.Warn("resolve launcher directory for install location check", "error", err)
	} else {
		closed = append(closed, filepath.Dir(exe))
	}
	limits := dirLimits{roots: make([]string, 0, len(roots))}
	for _, dir := range closed {
		if dir != "" {
			limits.closed = append(limits.closed, dir)
		}
	}
	for _, root := range roots {
		if root != "" {
			limits.roots = append(limits.roots, root)
		}
	}
	return limits
}

// entryDirs — второй источник каталога установки рядом со снимком: установщик
// мог поставить игру куда угодно, а запись в реестре называет, куда именно.
// Снимок остаётся основным, поэтому нечитаемый реестр не валит установку, а
// только логируется: setRemoval отметит UninstallUnknown по той же причине.
func (s *Service) entryDirs(id string, item Installation, roots []string, before map[string]uninstallEntry) []string {
	after, err := s.readEntries()
	if err != nil {
		slog.Error("read uninstall entries for install location", "id", id, "error", err)
		return nil
	}
	return installDirsFromEntries(before, after, item.Name, s.dirLimits(roots))
}

func installDirsFromEntries(before, after map[string]uninstallEntry, name string, limits dirLimits) []string {
	type located struct {
		entry uninstallEntry
		dir   string
	}
	found := make([]located, 0, 2)
	for _, entry := range newEntries(before, after) {
		if dir, ok := entry.installDir(limits); ok {
			found = append(found, located{entry, dir})
		}
	}
	if key := titleKey(name); key != "" && len(found) > 1 {
		named := make([]located, 0, len(found))
		for _, f := range found {
			if titleMatches(f.entry.DisplayName, key) {
				named = append(named, f)
			}
		}
		if len(named) > 0 {
			found = named
		}
	}
	dirs := make([]string, 0, len(found))
	for _, f := range found {
		dirs = append(dirs, f.dir)
	}
	sort.Strings(dirs)
	return shallowest(dirs)
}

func (e uninstallEntry) installDir(limits dirLimits) (string, bool) {
	if e.InstallLocation != "" {
		return usableDir(e.InstallLocation, limits)
	}
	for _, command := range []string{e.Command, e.QuietCommand} {
		path, _, err := splitCommand(command)
		if err != nil {
			continue
		}
		if dir, ok := usableDir(filepath.Dir(path), limits); ok {
			return dir, true
		}
	}
	if path := iconPath(e.Icon); path != "" {
		return usableDir(filepath.Dir(path), limits)
	}
	return "", false
}

func usableDir(raw string, limits dirLimits) (string, bool) {
	dir := strings.Trim(strings.TrimSpace(raw), `"`)
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) == dir {
		return "", false
	}
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		slog.Warn("stat install location from uninstall entry", "path", dir, "error", err)
		return "", false
	}
	if !info.IsDir() || !limits.allows(dir) {
		return "", false
	}
	return dir, true
}

// iconPath выделяет файл из DisplayIcon: значение бывает `"путь",0`, `путь,-101`
// и просто путём.
func iconPath(icon string) string {
	icon = strings.TrimSpace(icon)
	if strings.HasPrefix(icon, `"`) {
		end := strings.Index(icon[1:], `"`)
		if end < 0 {
			return ""
		}
		return icon[1 : end+1]
	}
	if comma := strings.LastIndex(icon, ","); comma >= 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(icon[comma+1:])); err == nil {
			return icon[:comma]
		}
	}
	return icon
}
