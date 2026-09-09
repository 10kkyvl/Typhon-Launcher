package install

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	highConfidenceScore = 60
	highConfidenceGap   = 20
	maxCandidates       = 12
)

var excludedPrefixes = []string{
	"setup",
	"install",
	"update",
	"cleanup",
	"repair",
	"config",
}

var excludedParts = []string{
	"unins",
	"uninstall",
	"setup",
	"crashreport",
	"crashhandler",
	"crashsender",
	"crashpad",
	"unitycrash",
	"subprocess",
	"redist",
	"prereq",
	"dxsetup",
	"dxwebsetup",
	"oalinst",
	"directx",
	"dotnet",
	"vcredist",
	"vc_redist",
	"vcruntime",
	"anticheat",
	"battleye",
	"beservice",
	"punkbuster",
	"webview",
	"activation",
	"helper",
	"dwtrig",
}

// skippedDirs — каталоги, в которых игры не бывает: распространяемые пакеты,
// античит, временные папки установщика. Их обход не только даёт ложных
// кандидатов, но и стоит времени на больших установках.
var skippedDirs = map[string]bool{
	"commonredist":     true,
	"redist":           true,
	"redists":          true,
	"directx":          true,
	"dotnet":           true,
	"dotnetfx":         true,
	"vcredist":         true,
	"prerequisites":    true,
	"prereq":           true,
	"prereqs":          true,
	"pluginsdir":       true,
	"easyanticheat":    true,
	"easyanticheateos": true,
	"battleye":         true,
	"punkbuster":       true,
	"uninstall":        true,
	"uninstaller":      true,
}

var binDirs = map[string]bool{
	"bin":      true,
	"binaries": true,
	"win64":    true,
	"win32":    true,
	"x64":      true,
	"x86":      true,
	"game":     true,
}

// bits64Dirs и bits32Dirs разводят сборки, которые лежат рядом: у игры с
// bin/x86 и bin/x64 обе одинаково похожи на название, и без этого различия
// выбор между ними достаётся сортировке по пути.
var bits64Dirs = map[string]bool{
	"win64": true,
	"x64":   true,
	"amd64": true,
	"bin64": true,
}

var bits32Dirs = map[string]bool{
	"win32": true,
	"x86":   true,
	"bin32": true,
}

var archSuffixes = []string{
	"win64shipping",
	"win32shipping",
	"shipping",
	"win64",
	"win32",
	"x64",
	"x86",
	"64",
	"32",
}

var assetExts = map[string]bool{
	".dll":    true,
	".pak":    true,
	".dat":    true,
	".bank":   true,
	".assets": true,
	".arc":    true,
	".bnk":    true,
	".pck":    true,
	".vpk":    true,
	".rpf":    true,
	".big":    true,
	".ress":   true,
}

var assetDirs = map[string]bool{
	"data":    true,
	"assets":  true,
	"content": true,
	"bin":     true,
	"engine":  true,
	"game":    true,
	"res":     true,
	"media":   true,
	"sound":   true,
	"audio":   true,
	"movies":  true,
	"locale":  true,
}

type exeFile struct {
	path  string
	rel   string
	base  string
	dir   string
	size  int64
	depth int
}

// paired — базовые имена, для которых в каталоге лежит данные движка:
// <Имя>_Data у Unity, <Имя>.pck у Godot. Такая пара опознаёт исполняемый файл
// игры надёжнее любого сходства с названием.
type paired map[string]map[string]bool

func (p paired) add(dir, base string) {
	if base == "" {
		return
	}
	set, ok := p[dir]
	if !ok {
		set = map[string]bool{}
		p[dir] = set
	}
	set[base] = true
}

func (p paired) has(dir, base string) bool {
	return p[dir][strings.ToLower(base)]
}

func FindExecutables(ctx context.Context, root, title string) ([]Candidate, error) {
	wanted := normalizeName(title)
	files := make([]exeFile, 0, 16)
	pairs := paired{}
	unreadable := 0
	var firstErr error

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			// Нечитаемый подкаталог (права, антивирус, битая точка
			// монтирования) не отменяет поиск по остальным: пропускаем его и
			// запоминаем, что осмотр вышел неполным.
			unreadable++
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("scan executables: directory skipped", "path", path, "error", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := d.Name()
		lower := strings.ToLower(name)
		if d.IsDir() {
			if path == root {
				return nil
			}
			if base, ok := strings.CutSuffix(lower, "_data"); ok {
				pairs.add(filepath.Dir(path), base)
				return fs.SkipDir
			}
			if skippedDirs[normalizeName(name)] {
				return fs.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(lower)
		if ext == ".pck" {
			pairs.add(filepath.Dir(path), strings.TrimSuffix(lower, ext))
			return nil
		}
		if ext != ".exe" {
			return nil
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if excludedExe(base) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", path, err)
		}
		files = append(files, exeFile{
			path:  path,
			rel:   rel,
			base:  base,
			dir:   filepath.Dir(path),
			size:  info.Size(),
			depth: strings.Count(filepath.ToSlash(rel), "/"),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan executables %s: %w", root, walkErr)
	}
	// Пустой ответ после пропущенных каталогов означал бы «игры здесь нет», а
	// это неправда: часть папок просто не открылась.
	if len(files) == 0 && unreadable > 0 {
		return nil, fmt.Errorf("scan executables %s: %d directories unreadable: %w", root, unreadable, firstErr)
	}

	shallowest := shallowestByName(files)
	out := make([]Candidate, 0, len(files))
	for _, f := range files {
		out = append(out, Candidate{Path: f.path, Score: scoreExe(f, wanted, pairs, shallowest)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

// LooksInstalled отвечает на вопрос «этот каталог — установленная игра»:
// Inspect тем же правилом опознаёт портируемую сборку, но до него добирается
// только после проверок на установщик и архив, которые для уже установленной
// игры дают ложный ответ.
func LooksInstalled(ctx context.Context, dir, title string) ([]Candidate, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, fmt.Errorf("read dir %s: %w", dir, err)
	}
	dirCount := 0
	for _, e := range entries {
		if e.IsDir() {
			dirCount++
		}
	}
	candidates, err := FindExecutables(ctx, dir, title)
	if err != nil {
		return nil, false, err
	}
	assets, err := hasAssets(dir)
	if err != nil {
		return nil, false, err
	}
	return candidates, installedLayout(candidates, dirCount, assets), nil
}

func installedLayout(candidates []Candidate, dirCount int, assets bool) bool {
	return len(candidates) > 0 && (dirCount > 0 || assets)
}

func HighConfidence(c []Candidate) bool {
	if len(c) == 0 {
		return false
	}
	if c[0].Score < highConfidenceScore {
		return false
	}
	if len(c) == 1 {
		return true
	}
	return c[0].Score-c[1].Score >= highConfidenceGap
}

// shallowestByName запоминает, на какой глубине лежит самый верхний файл с
// таким именем. Второй экземпляр той же программы глубже в дереве — это
// приложенный к игре служебный агент, а не сама игра: у Retro Gadgets рядом
// с RG.exe лежит PlaygroundAgent/RG.exe со своими данными движка, и по всем
// остальным признакам они неразличимы.
func shallowestByName(files []exeFile) map[string]int {
	out := make(map[string]int, len(files))
	for _, f := range files {
		name := strings.ToLower(f.base)
		if depth, ok := out[name]; !ok || f.depth < depth {
			out[name] = f.depth
		}
	}
	return out
}

func scoreExe(f exeFile, wanted string, pairs paired, shallowest map[string]int) float64 {
	parts := strings.Split(filepath.ToSlash(f.rel), "/")
	depth := len(parts) - 1

	score := math.Max(0, 40-float64(depth)*14)
	for _, part := range parts[:depth] {
		if binDirs[strings.ToLower(part)] {
			score += 18
			break
		}
	}
	for _, part := range parts[:depth] {
		lower := strings.ToLower(part)
		if bits64Dirs[lower] {
			score += 6
			break
		}
		if bits32Dirs[lower] {
			score -= 6
			break
		}
	}
	flat := normalizeName(f.base)
	score += math.Max(similarity(flat, wanted), similarity(trimArch(flat), wanted))
	if pairs.has(f.dir, f.base) {
		score += 30
	}
	if strings.HasSuffix(flat, "shipping") {
		score += 20
	}
	if top, ok := shallowest[strings.ToLower(f.base)]; ok && depth > top {
		score -= 12
	}
	if f.size > 0 {
		score += math.Min(30, math.Log10(float64(f.size)/1024+1)*8)
	}
	return math.Round(score*100) / 100
}

// trimArch убирает у имени хвост сборки: Game-Win64-Shipping и Game_x64 — то
// же самое имя, что и Game, а сходство с названием считается по буквам.
func trimArch(flat string) string {
	for {
		trimmed := flat
		for _, suffix := range archSuffixes {
			if len(trimmed) > len(suffix) {
				trimmed = strings.TrimSuffix(trimmed, suffix)
			}
		}
		if trimmed == flat {
			return flat
		}
		flat = trimmed
	}
}

func similarity(name, wanted string) float64 {
	if name == "" || wanted == "" {
		return 0
	}
	if name == wanted {
		return 45
	}
	if strings.Contains(wanted, name) || strings.Contains(name, wanted) {
		return 30
	}
	shared := commonPrefix(name, wanted)
	if shared >= 4 {
		return math.Min(20, float64(shared)*3)
	}
	return 0
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 'а' && r <= 'я') || r == 'ё' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func excludedExe(base string) bool {
	name := strings.ToLower(base)
	flat := normalizeName(base)
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, p := range excludedParts {
		if strings.Contains(name, p) || strings.Contains(flat, p) {
			return true
		}
	}
	return false
}

func hasAssets(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", root, err)
	}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if e.IsDir() {
			if assetDirs[name] || strings.HasSuffix(name, "_data") || strings.HasSuffix(name, "content") {
				return true, nil
			}
			continue
		}
		if assetExts[strings.ToLower(filepath.Ext(name))] {
			return true, nil
		}
	}
	return false, nil
}
