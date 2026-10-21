package settings

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	ErrExtraLibraryPathNested  = uierr.New("settings.extra_library_nested", "дополнительная папка не может лежать внутри папки библиотеки или содержать её")
	ErrExtraLibraryPathUnknown = errors.New("такой дополнительной папки нет в списке")
)

// LibraryFolder is one place games are installed to. Available is false when
// the volume holding it is not there at the moment (a disconnected drive), so
// the screen can grey the entry out instead of failing the whole list.
type LibraryFolder struct {
	Path       string `json:"path"`
	Primary    bool   `json:"primary"`
	FreeBytes  int64  `json:"freeBytes"`
	TotalBytes int64  `json:"totalBytes"`
	Available  bool   `json:"available"`
}

// GamesRoots lists every directory games are installed into: the games folder
// of the main library first, then the extra folders. The extra folders are
// roots of their own, a game lives directly at <folder>\<name>.
func (s Settings) GamesRoots() []string {
	roots := make([]string, 0, 1+len(s.ExtraLibraryPaths))
	if s.GamesPath != "" {
		roots = append(roots, s.GamesPath)
	}
	return append(roots, s.ExtraLibraryPaths...)
}

func sanitizeExtraLibraryPaths(s Settings) (Settings, error) {
	own := []string{s.LibraryPath, s.GamesPath, s.DownloadsPath}
	out := make([]string, 0, len(s.ExtraLibraryPaths))
	keys := make([]string, 0, len(s.ExtraLibraryPaths))
	for _, raw := range s.ExtraLibraryPaths {
		path, err := normalizeLibraryPath(raw)
		if err != nil {
			return Settings{}, err
		}
		if path == "" {
			continue
		}
		key, err := platform.PathKey(path)
		if err != nil {
			return Settings{}, fmt.Errorf("extra library path %s: %w", path, err)
		}
		if slices.Contains(keys, key) {
			continue
		}
		for _, other := range own {
			if other != "" && (platform.Inside(other, path) || platform.Inside(path, other)) {
				return Settings{}, fmt.Errorf("%w: %s", ErrExtraLibraryPathNested, path)
			}
		}
		for _, other := range out {
			if platform.Inside(other, path) || platform.Inside(path, other) {
				return Settings{}, fmt.Errorf("%w: %s", ErrExtraLibraryPathNested, path)
			}
		}
		keys = append(keys, key)
		out = append(out, path)
	}
	s.ExtraLibraryPaths = out
	s.LastInstallRoot = knownRoot(s, s.LastInstallRoot)
	return s, nil
}

func knownRoot(s Settings, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	for _, root := range s.GamesRoots() {
		if platform.SamePath(root, path) {
			return root
		}
	}
	return ""
}

// AddLibraryFolder makes path one more place games can be installed to. A
// folder that does not exist is created, and one this call created is removed
// again if the folder turns out unusable or the list cannot be saved.
func (s *Service) AddLibraryFolder(ctx context.Context, path string) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	folder, err := normalizeLibraryPath(path)
	if err != nil {
		return Settings{}, err
	}
	if folder == "" {
		return Settings{}, ErrLibraryParentEmpty
	}
	probe := s.GetSettings()
	probe.ExtraLibraryPaths = append(slices.Clone(probe.ExtraLibraryPaths), folder)
	if _, err := sanitizeExtraLibraryPaths(probe); err != nil {
		return Settings{}, err
	}

	created, err := createFolder(folder)
	if err != nil {
		return Settings{}, errors.Join(err, removeCreated(created))
	}
	if err := ctx.Err(); err != nil {
		return Settings{}, errors.Join(err, removeCreated(created))
	}
	saved, err := s.Update(func(next *Settings) error {
		next.ExtraLibraryPaths = append(slices.Clone(next.ExtraLibraryPaths), folder)
		return nil
	})
	if err != nil {
		return Settings{}, errors.Join(err, removeCreated(created))
	}
	return saved, nil
}

// RemoveLibraryFolder takes a folder off the list and nothing else: the files
// stay where they are, and games installed there stay in the library by their
// own install directory.
func (s *Service) RemoveLibraryFolder(ctx context.Context, path string) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	folder, err := normalizeLibraryPath(path)
	if err != nil {
		return Settings{}, err
	}
	if folder == "" {
		return Settings{}, ErrLibraryParentEmpty
	}
	return s.Update(func(next *Settings) error {
		kept := make([]string, 0, len(next.ExtraLibraryPaths))
		for _, extra := range next.ExtraLibraryPaths {
			if !platform.SamePath(extra, folder) {
				kept = append(kept, extra)
			}
		}
		if len(kept) == len(next.ExtraLibraryPaths) {
			return fmt.Errorf("%w: %s", ErrExtraLibraryPathUnknown, folder)
		}
		next.ExtraLibraryPaths = kept
		return nil
	})
}

// LibraryFolders reports every games root with the room left on its volume.
// A volume that is not there is listed as unavailable; any other failure to
// read the space is an error, not a folder with no room.
func (s *Service) LibraryFolders(ctx context.Context) ([]LibraryFolder, error) {
	cfg := s.GetSettings()
	folders := make([]LibraryFolder, 0, 1+len(cfg.ExtraLibraryPaths))
	for i, root := range cfg.GamesRoots() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		folder := LibraryFolder{Path: root, Primary: i == 0 && cfg.GamesPath != ""}
		info, err := s.storageInfo(root)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("свободное место в %s: %w", root, err)
		default:
			folder.Available = true
			folder.FreeBytes = clampBytes(info.FreeBytes)
			folder.TotalBytes = clampBytes(info.TotalBytes)
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

func clampBytes(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func createFolder(dir string) ([]string, error) {
	var missing []string
	for cur := dir; ; {
		_, err := os.Stat(cur)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, uierr.Wrap("settings.library_create_failed", fmt.Errorf("проверить папку %s: %w", cur, err))
		}
		missing = append(missing, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	slices.Reverse(missing)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return missing, uierr.Wrap("settings.library_create_failed", fmt.Errorf("создать папку %s: %w", dir, err))
	}
	if err := checkWritable(dir); err != nil {
		return missing, uierr.Wrap("settings.library_not_writable", err)
	}
	return missing, nil
}

// os.Remove, not RemoveAll: anything that appeared inside meanwhile is kept.
func removeCreated(created []string) error {
	var failed []error
	for i := len(created) - 1; i >= 0; i-- {
		if err := os.Remove(created[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			failed = append(failed, fmt.Errorf("убрать созданную папку %s: %w", created[i], err))
		}
	}
	return errors.Join(failed...)
}
