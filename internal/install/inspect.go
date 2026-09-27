package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"typhon/internal/download"
	"typhon/internal/uierr"
)

const (
	junkFileLimit    = 1 << 20
	maxDescendDepth  = 3
	dominantFraction = 0.8
)

var errNoSource = uierr.New("install.no_source", "папка загрузки недоступна")
var errEmptySource = uierr.New("install.empty_source", "папка загрузки пуста — файлы не были загружены")
var errIncompleteSource = uierr.New("install.incomplete_source", "загрузка не завершена — на диске остались только незавершённые файлы")
var errUnrecognizedSource = uierr.New("install.unrecognized_source", "формат файла не распознан")
var errMixedInstallers = uierr.New("install.mixed_installers", "установщики набора разного типа")

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
	if len(entries) == 0 {
		return Plan{}, fmt.Errorf("%w: %s", errEmptySource, root)
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
	if len(files) > 0 && allPartial(files) {
		return Plan{}, fmt.Errorf("%w: %s", errIncompleteSource, root)
	}

	total, err := DirSize(ctx, root)
	if err != nil {
		return Plan{}, err
	}
	plan.EstimatedSize = total

	if msi := pickByExt(root, files, ".msi"); msi != "" {
		if err := fillInstallerPlan(&plan, TypeMsiInstaller, msi); err != nil {
			return Plan{}, err
		}
		return plan, nil
	}

	if exes := pickInstallerExes(root, files); len(exes) > 0 {
		if err := fillInstallerPlan(&plan, TypeExeInstaller, exes[0]); err != nil {
			return Plan{}, err
		}
		if err := fillInstallerChain(&plan, exes[1:]); err != nil {
			return Plan{}, err
		}
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
	if installedLayout(candidates, dirCount, assets) {
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
		if err := fillInstallerPlan(&plan, TypeMsiInstaller, path); err != nil {
			return Plan{}, err
		}
		plan.EstimatedSize = size
	case ".exe":
		if err := fillInstallerPlan(&plan, TypeExeInstaller, path); err != nil {
			return Plan{}, err
		}
		plan.EstimatedSize = size
	default:
		return Plan{}, fmt.Errorf("%w: %s", errUnrecognizedSource, filepath.Base(path))
	}
	return plan, nil
}

func fillInstallerPlan(plan *Plan, kind Type, installer string) error {
	engine, err := DetectEngine(installer)
	if err != nil {
		return err
	}
	plan.Type = kind
	plan.InstallerPath = installer
	plan.ExtraInstallers = nil
	plan.WorkingDir = filepath.Dir(installer)
	plan.Engine = engine
	plan.Silent = supportsSilent(engine)
	plan.CanAutoInstall = plan.Silent
	plan.RequiresUserInteraction = !plan.Silent
	return nil
}

func fillInstallerChain(plan *Plan, extras []string) error {
	for _, extra := range extras {
		engine, err := DetectEngine(extra)
		if err != nil {
			return err
		}
		if engine != plan.Engine {
			return fmt.Errorf("%w: %s", errMixedInstallers, filepath.Base(extra))
		}
		plan.ExtraInstallers = append(plan.ExtraInstallers, extra)
	}
	return nil
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

func allPartial(files []sizedEntry) bool {
	for _, f := range files {
		if !download.IsPartFile(f.name) {
			return false
		}
	}
	return true
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

type installerEntry struct {
	name    string
	stem    string
	slug    string
	payload int64
	depth   int
}

func pickInstallerExes(root string, files []sizedEntry) []string {
	var exes, data, named []sizedEntry
	for _, f := range files {
		lower := strings.ToLower(f.name)
		if strings.HasSuffix(lower, ".exe") {
			exes = append(exes, f)
			if strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install") {
				named = append(named, f)
			}
			continue
		}
		if installerDataExts[filepath.Ext(lower)] {
			data = append(data, f)
		}
	}
	if len(named) == 0 {
		if len(exes) == 1 && len(data) > 0 {
			return []string{filepath.Join(root, exes[0].name)}
		}
		return nil
	}
	entries := orderInstallers(named, data)
	out := []string{filepath.Join(root, entries[0].name)}
	for _, e := range entries[1:] {
		if dependsOn(e.slug, entries[0].slug) {
			out = append(out, filepath.Join(root, e.name))
		}
	}
	return out
}

// orderInstallers ставит первым базовый установщик набора: каталог загрузки GOG
// содержит и игру, и дополнения, а установщик дополнения без установленной игры
// завершается с кодом 1 ещё в InitializeSetup.
func orderInstallers(exes, data []sizedEntry) []installerEntry {
	entries := make([]installerEntry, 0, len(exes))
	for _, f := range exes {
		stem := strings.ToLower(strings.TrimSuffix(f.name, filepath.Ext(f.name)))
		entries = append(entries, installerEntry{name: f.name, stem: stem, slug: installerSlug(stem), payload: f.size})
	}
	for _, f := range data {
		lower := strings.ToLower(f.name)
		best := -1
		for i := range entries {
			if !strings.HasPrefix(lower, entries[i].stem) {
				continue
			}
			if best < 0 || len(entries[i].stem) > len(entries[best].stem) {
				best = i
			}
		}
		if best >= 0 {
			entries[best].payload += f.size
		}
	}
	for i := range entries {
		for j := range entries {
			if i != j && dependsOn(entries[i].slug, entries[j].slug) {
				entries[i].depth++
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].depth != entries[j].depth {
			return entries[i].depth < entries[j].depth
		}
		if entries[i].payload != entries[j].payload {
			return entries[i].payload > entries[j].payload
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func dependsOn(slug, base string) bool {
	return base != "" && len(slug) > len(base) && strings.HasPrefix(slug, base+"_")
}

func installerSlug(stem string) string {
	s := stem
	for strings.HasSuffix(s, ")") {
		open := strings.LastIndex(s, "(")
		if open <= 0 || s[open-1] != '_' {
			break
		}
		s = s[:open-1]
	}
	if i := strings.LastIndex(s, "_"); i > 0 && versionToken(s[i+1:]) {
		s = s[:i]
	}
	return s
}

func versionToken(t string) bool {
	if t == "" || t[0] < '0' || t[0] > '9' {
		return false
	}
	for _, r := range t {
		if (r >= '0' && r <= '9') || r == '.' || (r >= 'a' && r <= 'z') {
			continue
		}
		return false
	}
	return true
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
