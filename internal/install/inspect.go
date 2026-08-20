package install

import (
	"errors"
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

func Inspect(dir string) (Plan, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Plan{}, errNoSource
	}
	if !info.IsDir() {
		return inspectFile(dir, info.Size()), nil
	}

	root := normalizeRoot(dir)
	plan := Plan{
		Type:        TypeUnknown,
		SourcePath:  dir,
		ContentRoot: root,
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return Plan{}, errNoSource
	}

	var files []os.DirEntry
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
			continue
		}
		files = append(files, e)
	}

	if msi := pickByExt(root, files, ".msi"); msi != "" {
		plan.Type = TypeMsiInstaller
		plan.InstallerPath = msi
		plan.WorkingDir = filepath.Dir(msi)
		plan.EstimatedSize = DirSize(root)
		plan.RequiresUserInteraction = true
		return plan, nil
	}

	if exe := pickInstallerExe(root, files); exe != "" {
		plan.Type = TypeExeInstaller
		plan.InstallerPath = exe
		plan.WorkingDir = filepath.Dir(exe)
		plan.EstimatedSize = DirSize(root)
		plan.RequiresUserInteraction = true
		return plan, nil
	}

	if archive := pickDominantArchive(root, files); archive != "" {
		fillArchivePlan(&plan, archive)
		return plan, nil
	}

	title := filepath.Base(dir)
	candidates := FindExecutables(root, title)
	if len(candidates) > 0 && (len(dirs) > 0 || hasAssets(root)) {
		plan.Type = TypePortable
		plan.EstimatedSize = DirSize(root)
		plan.Candidates = candidates
		plan.CanAutoInstall = true
		return plan, nil
	}

	plan.EstimatedSize = DirSize(root)
	plan.Candidates = candidates
	plan.RequiresUserInteraction = true
	return plan, nil
}

func inspectFile(path string, size int64) Plan {
	plan := Plan{
		Type:        TypeUnknown,
		SourcePath:  path,
		ContentRoot: filepath.Dir(path),
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".7z", ".rar":
		fillArchivePlan(&plan, path)
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
	return plan
}

func fillArchivePlan(plan *Plan, archive string) {
	plan.Type = archiveType(archive)
	plan.ArchivePath = archive
	plan.CanAutoInstall = true
	if info, err := os.Stat(archive); err == nil {
		plan.CompressedSize = info.Size()
	}
	if size, err := EstimateExtracted(archive); err == nil {
		plan.EstimatedSize = size
	}
}

func normalizeRoot(dir string) string {
	current := dir
	for i := 0; i < maxDescendDepth; i++ {
		entries, err := os.ReadDir(current)
		if err != nil {
			return current
		}
		var sub string
		subCount := 0
		meaningful := 0
		for _, e := range entries {
			if e.IsDir() {
				subCount++
				sub = e.Name()
				continue
			}
			if !isJunk(e) {
				meaningful++
			}
		}
		if subCount != 1 || meaningful != 0 {
			return current
		}
		current = filepath.Join(current, sub)
	}
	return current
}

func isJunk(e os.DirEntry) bool {
	name := e.Name()
	if strings.HasPrefix(name, ".") {
		return true
	}
	if !junkExts[strings.ToLower(filepath.Ext(name))] {
		return false
	}
	info, err := e.Info()
	return err == nil && info.Size() <= junkFileLimit
}

func pickByExt(root string, files []os.DirEntry, ext string) string {
	var best string
	var bestSize int64 = -1
	for _, f := range files {
		if !strings.EqualFold(filepath.Ext(f.Name()), ext) {
			continue
		}
		size := entrySize(f)
		if size > bestSize {
			best, bestSize = filepath.Join(root, f.Name()), size
		}
	}
	return best
}

func pickInstallerExe(root string, files []os.DirEntry) string {
	var exes []os.DirEntry
	hasData := false
	for _, f := range files {
		lower := strings.ToLower(f.Name())
		if strings.HasSuffix(lower, ".exe") {
			exes = append(exes, f)
			continue
		}
		if installerDataExts[filepath.Ext(lower)] {
			hasData = true
		}
	}
	for _, f := range exes {
		lower := strings.ToLower(f.Name())
		if strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install") {
			return filepath.Join(root, f.Name())
		}
	}
	if len(exes) == 1 && hasData {
		return filepath.Join(root, exes[0].Name())
	}
	return ""
}

func pickDominantArchive(root string, files []os.DirEntry) string {
	var archives []os.DirEntry
	meaningful := 0
	for _, f := range files {
		if IsArchive(f.Name()) {
			archives = append(archives, f)
		}
		if !isJunk(f) {
			meaningful++
		}
	}
	if len(archives) == 0 {
		return ""
	}
	sort.Slice(archives, func(i, j int) bool { return entrySize(archives[i]) > entrySize(archives[j]) })
	largest := archives[0]
	path := filepath.Join(root, largest.Name())

	if meaningful == 1 && len(archives) == 1 {
		return path
	}
	total := DirSize(root)
	if total > 0 && float64(entrySize(largest)) >= float64(total)*dominantFraction {
		return path
	}
	return ""
}

func entrySize(e os.DirEntry) int64 {
	info, err := e.Info()
	if err != nil {
		return 0
	}
	return info.Size()
}
