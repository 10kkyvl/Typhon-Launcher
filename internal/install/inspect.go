package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	junkFileLimit    = 1 << 20
	maxDescendDepth  = 3
	dominantFraction = 0.8
)

var errNoSource = errors.New("папка загрузки недоступна")

var junkExts = map[string]bool{
	".nfo": true,
	".txt": true,
	".url": true,
	".sfv": true,
	".md5": true,
	".diz": true,
	".log": true,
	".ini": true,
}

var installerDataExts = map[string]bool{
	".bin": true,
	".cab": true,
	".hdr": true,
	".mst": true,
	".ins": true,
	".gid": true,
}

type sizedEntry struct {
	name  string
	size  int64
	isDir bool
}

func sizedEntries(dir string) ([]sizedEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make([]sizedEntry, 0, len(entries))
	for _, e := range entries {
		item := sizedEntry{name: e.Name(), isDir: e.IsDir()}
		if !item.isDir {
			info, err := e.Info()
			if err != nil {
				return nil, fmt.Errorf("stat %s: %w", filepath.Join(dir, e.Name()), err)
			}
			item.size = info.Size()
		}
		out = append(out, item)
	}
	return out, nil
}

func Inspect(ctx context.Context, dir string) (Plan, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %w", errNoSource, err)
	}
	if !info.IsDir() {
		return inspectFile(dir, info.Size())
	}

	root, err := normalizeRoot(dir)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Type:        TypeUnknown,
		SourcePath:  dir,
		ContentRoot: root,
	}

	entries, err := sizedEntries(root)
	if err != nil {
		return Plan{}, err
	}
	var files []sizedEntry
	dirCount := 0
	for _, e := range entries {
		if e.isDir {
			dirCount++
			continue
		}
		files = append(files, e)
	}

	total, err := DirSize(ctx, root)
	if err != nil {
		return Plan{}, err
	}
	plan.EstimatedSize = total

	if msi := pickByExt(root, files, ".msi"); msi != "" {
		plan.Type = TypeMsiInstaller
		plan.InstallerPath = msi
		plan.WorkingDir = filepath.Dir(msi)
		plan.RequiresUserInteraction = true
		return plan, nil
	}

	if exe := pickInstallerExe(root, files); exe != "" {
		plan.Type = TypeExeInstaller
		plan.InstallerPath = exe
		plan.WorkingDir = filepath.Dir(exe)
		plan.RequiresUserInteraction = true
		return plan, nil
	}

	if archive := pickDominantArchive(root, files, total); archive != "" {
		if err := fillArchivePlan(&plan, archive); err != nil {
			return Plan{}, err
		}
		return plan, nil
	}

	title := filepath.Base(dir)
	candidates, err := FindExecutables(ctx, root, title)
	if err != nil {
		return Plan{}, err
	}
	assets, err := hasAssets(root)
	if err != nil {
		return Plan{}, err
	}
	plan.Candidates = candidates
	if len(candidates) > 0 && (dirCount > 0 || assets) {
		plan.Type = TypePortable
		plan.CanAutoInstall = true
		return plan, nil
	}

	plan.RequiresUserInteraction = true
	return plan, nil
}

func inspectFile(path string, size int64) (Plan, error) {
	plan := Plan{
		Type:        TypeUnknown,
		SourcePath:  path,
		ContentRoot: filepath.Dir(path),
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".7z", ".rar":
		if err := fillArchivePlan(&plan, path); err != nil {
			return Plan{}, err
		}
	case ".msi":
		plan.Type = TypeMsiInstaller
		plan.InstallerPath = path
		plan.WorkingDir = filepath.Dir(path)
		plan.EstimatedSize = size
		plan.RequiresUserInteraction = true
	case ".exe":
		plan.Type = TypeExeInstaller
		plan.InstallerPath = path
		plan.WorkingDir = filepath.Dir(path)
		plan.EstimatedSize = size
		plan.RequiresUserInteraction = true
	default:
		plan.EstimatedSize = size
		plan.RequiresUserInteraction = true
	}
	return plan, nil
}

func fillArchivePlan(plan *Plan, archive string) error {
	info, err := os.Stat(archive)
	if err != nil {
		return fmt.Errorf("stat %s: %w", archive, err)
	}
	size, err := EstimateExtracted(archive)
	if err != nil {
		return fmt.Errorf("estimate %s: %w", archive, err)
	}
	plan.Type = archiveType(archive)
	plan.ArchivePath = archive
	plan.CompressedSize = info.Size()
	plan.EstimatedSize = size
	plan.CanAutoInstall = true
	return nil
}

func normalizeRoot(dir string) (string, error) {
	current := dir
	for i := 0; i < maxDescendDepth; i++ {
		entries, err := sizedEntries(current)
		if err != nil {
			return "", err
		}
		var sub string
		subCount := 0
		meaningful := 0
		for _, e := range entries {
			if e.isDir {
				subCount++
				sub = e.name
				continue
			}
			if !isJunk(e) {
				meaningful++
			}
		}
		if subCount != 1 || meaningful != 0 {
			return current, nil
		}
		current = filepath.Join(current, sub)
	}
	return current, nil
}

func isJunk(e sizedEntry) bool {
	if strings.HasPrefix(e.name, ".") {
		return true
	}
	if !junkExts[strings.ToLower(filepath.Ext(e.name))] {
		return false
	}
	return e.size <= junkFileLimit
}

func pickByExt(root string, files []sizedEntry, ext string) string {
	var best string
	var bestSize int64 = -1
	for _, f := range files {
		if !strings.EqualFold(filepath.Ext(f.name), ext) {
			continue
		}
		if f.size > bestSize {
			best, bestSize = filepath.Join(root, f.name), f.size
		}
	}
	return best
}

func pickInstallerExe(root string, files []sizedEntry) string {
	var exes []sizedEntry
	hasData := false
	for _, f := range files {
		lower := strings.ToLower(f.name)
		if strings.HasSuffix(lower, ".exe") {
			exes = append(exes, f)
			continue
		}
		if installerDataExts[filepath.Ext(lower)] {
			hasData = true
		}
	}
	for _, f := range exes {
		lower := strings.ToLower(f.name)
		if strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install") {
			return filepath.Join(root, f.name)
		}
	}
	if len(exes) == 1 && hasData {
		return filepath.Join(root, exes[0].name)
	}
	return ""
}

func pickDominantArchive(root string, files []sizedEntry, total int64) string {
	var archives []sizedEntry
	meaningful := 0
	for _, f := range files {
		if IsArchive(f.name) {
			archives = append(archives, f)
		}
		if !isJunk(f) {
			meaningful++
		}
	}
	if len(archives) == 0 {
		return ""
	}
	sort.Slice(archives, func(i, j int) bool { return archives[i].size > archives[j].size })
	largest := archives[0]
	path := filepath.Join(root, largest.name)

	if meaningful == 1 && len(archives) == 1 {
		return path
	}
	if total > 0 && float64(largest.size) >= float64(total)*dominantFraction {
		return path
	}
	return ""
}
