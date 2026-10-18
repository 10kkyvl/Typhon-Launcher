package redist

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"typhon/internal/platform"
)

const (
	KindVCRedist = "vcredist"
	KindDirectX  = "directx"
	KindDotNet   = "dotnet"
	KindOther    = "other"

	maxScanDepth = 4

	exitSuccess        = 0
	exitRebootRequired = 3010
	exitNewerInstalled = 1638
)

var (
	ErrElevationDeclined = errors.New("запрос прав администратора отклонён")
	ErrUnsupported       = errors.New("установка компонентов поддерживается только в Windows")
	ErrNoSilentArgs      = errors.New("для этого установщика нет известных тихих ключей")
	ErrEmptyInstallDir   = errors.New("не задана папка установки игры")
	ErrNotAbsolute       = errors.New("путь установщика должен быть абсолютным")
	ErrNotAFile          = errors.New("установщик не найден или это не файл")
	ErrNotRedist         = errors.New("файл не является распознанным компонентом")
	ErrOutsideInstall    = errors.New("путь выходит за папку игры")
	ErrLinkInPath        = errors.New("на пути к установщику есть символическая ссылка или точка повторной обработки")
	ErrRootNotDir        = errors.New("папка установки не является обычным каталогом")
	ErrCancelFailed      = errors.New("установщик продолжает работу, дождитесь его завершения")
)

type Package struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Runtime struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

type Result struct {
	ExitCode         int  `json:"exitCode"`
	RebootRequired   bool `json:"rebootRequired"`
	AlreadyInstalled bool `json:"alreadyInstalled"`
}

type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Arch string `json:"arch"`
}

type ExitError struct{ Code int }

func (e *ExitError) Error() string {
	return fmt.Sprintf("установщик завершился с кодом %d (0x%08X)", e.Code, uint32(int64(e.Code)&0xFFFFFFFF))
}

var redistDirs = map[string]struct{}{
	"_commonredist":    {},
	"commonredist":     {},
	"redist":           {},
	"_redist":          {},
	"redistributables": {},
	"vcredist":         {},
	"directx":          {},
	"dxsetup":          {},
	"dotnet":           {},
	"prerequisites":    {},
	"support":          {},
}

var filePatterns = []string{
	"vcredist*.exe",
	"vc_redist*.exe",
	"dxsetup.exe",
	"dxwebsetup.exe",
	"ndp*.exe",
	"dotnet*-runtime*.exe",
	"windowsdesktop-runtime*.exe",
	"oalinst.exe",
	"physx*.msi",
	"physx*.exe",
}

var yearPattern = regexp.MustCompile(`(?:^|[^0-9])(2005|2008|2010|2012|2013|2015|2017|2019|2022)(?:[^0-9]|$)`)

func OfficialLinks() []Link {
	return []Link{
		{Name: "Microsoft Visual C++ Redistributable (x64)", URL: "https://aka.ms/vs/17/release/vc_redist.x64.exe", Arch: "x64"},
		{Name: "Microsoft Visual C++ Redistributable (x86)", URL: "https://aka.ms/vs/17/release/vc_redist.x86.exe", Arch: "x86"},
		{Name: "DirectX End-User Runtime", URL: "https://www.microsoft.com/download/details.aspx?id=35", Arch: ""},
		{Name: ".NET", URL: "https://dotnet.microsoft.com/download", Arch: ""},
	}
}

func classify(name string) (string, bool) {
	lower := strings.ToLower(name)
	matched := false
	for _, pattern := range filePatterns {
		hit, err := path.Match(pattern, lower)
		if err != nil {
			return "", false
		}
		if hit {
			matched = true
			break
		}
	}
	if !matched {
		return "", false
	}
	switch {
	case strings.HasPrefix(lower, "vcredist"), strings.HasPrefix(lower, "vc_redist"):
		return KindVCRedist, true
	case strings.HasPrefix(lower, "dxsetup"), strings.HasPrefix(lower, "dxwebsetup"):
		return KindDirectX, true
	case strings.HasPrefix(lower, "ndp"), strings.HasPrefix(lower, "dotnet"), strings.HasPrefix(lower, "windowsdesktop"):
		return KindDotNet, true
	}
	return KindOther, true
}

func archOf(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "x64"), strings.Contains(lower, "amd64"):
		return "x64"
	case strings.Contains(lower, "x86"):
		return "x86"
	}
	return ""
}

func yearOf(s string) string {
	m := yearPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

func depth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return strings.Count(filepath.ToSlash(rel), "/") + 1
}

func underRedistDir(relDir string) bool {
	if relDir == "." || relDir == "" {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(relDir), "/") {
		if _, ok := redistDirs[strings.ToLower(part)]; ok {
			return true
		}
	}
	return false
}

func Recognize(installDir, file string) (Package, bool) {
	rel, err := filepath.Rel(installDir, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Package{}, false
	}
	return recognizeRel(rel, file)
}

func recognizeRel(rel, file string) (Package, bool) {
	dir := filepath.Dir(rel)
	if depth(dir) > maxScanDepth || !underRedistDir(dir) {
		return Package{}, false
	}
	name := filepath.Base(file)
	kind, ok := classify(name)
	if !ok {
		return Package{}, false
	}
	version := yearOf(name)
	if version == "" {
		version = yearOf(filepath.Base(dir))
	}
	return Package{Name: name, Path: file, Kind: kind, Arch: archOf(name), Version: version}, true
}

func isLink(info fs.FileInfo) bool {
	return info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0
}

func checkRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("папка установки %s: %w", root, err)
	}
	if isLink(info) || !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrRootNotDir, root)
	}
	return nil
}

func CheckInside(installDir, file string) error {
	if strings.TrimSpace(installDir) == "" {
		return ErrEmptyInstallDir
	}
	root := filepath.Clean(installDir)
	if err := checkRoot(root); err != nil {
		return err
	}
	if !platform.Inside(root, file) {
		return fmt.Errorf("%w: %s", ErrOutsideInstall, file)
	}
	rel, err := filepath.Rel(root, filepath.Clean(file))
	if err != nil {
		return fmt.Errorf("относительный путь %s: %w", file, err)
	}
	current := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("проверить %s: %w", current, err)
		}
		if isLink(info) {
			return fmt.Errorf("%w: %s", ErrLinkInPath, current)
		}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("разрешить %s: %w", root, err)
	}
	realFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		return fmt.Errorf("разрешить %s: %w", file, err)
	}
	if !platform.Inside(realRoot, realFile) {
		return fmt.Errorf("%w: %s", ErrOutsideInstall, realFile)
	}
	return nil
}

func cancelRun(ctxErr error, kill func() error) error {
	if err := kill(); err != nil {
		return fmt.Errorf("%w: %w", ErrCancelFailed, err)
	}
	return ctxErr
}

func Scan(ctx context.Context, installDir string) ([]Package, error) {
	if strings.TrimSpace(installDir) == "" {
		return nil, ErrEmptyInstallDir
	}
	root := filepath.Clean(installDir)
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	out := []Package{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if p != root && errors.Is(walkErr, fs.ErrNotExist) {
				slog.Warn("redist scan: entry vanished", "path", p, "error", walkErr)
				return nil
			}
			return fmt.Errorf("обход %s: %w", p, walkErr)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("относительный путь %s: %w", p, err)
		}
		if d.IsDir() {
			if depth(rel) > maxScanDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		pkg, ok := recognizeRel(rel, p)
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("размер %s: %w", p, err)
		}
		pkg.SizeBytes = info.Size()
		out = append(out, pkg)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func silentArgs(pkg Package) ([]string, error) {
	name := strings.ToLower(filepath.Base(pkg.Path))
	if strings.HasSuffix(name, ".msi") {
		return []string{"/i", pkg.Path, "/qn", "/norestart"}, nil
	}
	switch pkg.Kind {
	case KindVCRedist:
		year := yearOf(name)
		if year == "" {
			year = yearOf(filepath.Base(filepath.Dir(pkg.Path)))
		}
		switch year {
		case "2005", "2008":
			return []string{"/q"}, nil
		case "2010", "2012", "2013":
			return []string{"/q", "/norestart"}, nil
		}
		return []string{"/install", "/quiet", "/norestart"}, nil
	case KindDirectX:
		if strings.HasPrefix(name, "dxwebsetup") {
			return []string{"/Q"}, nil
		}
		return []string{"/silent"}, nil
	case KindDotNet:
		return []string{"/q", "/norestart"}, nil
	case KindOther:
		if name == "oalinst.exe" {
			return []string{"/s"}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoSilentArgs, filepath.Base(pkg.Path))
}

func interpretExit(code int) (Result, error) {
	switch code {
	case exitSuccess:
		return Result{ExitCode: code}, nil
	case exitRebootRequired:
		return Result{ExitCode: code, RebootRequired: true}, nil
	case exitNewerInstalled:
		return Result{ExitCode: code, AlreadyInstalled: true}, nil
	}
	return Result{ExitCode: code}, &ExitError{Code: code}
}

type runFunc func(ctx context.Context, exe string, args []string, workDir string) (int, error)

var run runFunc = runElevated

func Install(ctx context.Context, pkg Package) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !filepath.IsAbs(pkg.Path) {
		return Result{}, fmt.Errorf("%w: %s", ErrNotAbsolute, pkg.Path)
	}
	info, err := os.Stat(pkg.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, fmt.Errorf("%w: %s", ErrNotAFile, pkg.Path)
	}
	if err != nil {
		return Result{}, fmt.Errorf("проверить %s: %w", pkg.Path, err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("%w: %s", ErrNotAFile, pkg.Path)
	}
	args, err := silentArgs(pkg)
	if err != nil {
		return Result{}, err
	}
	exe := pkg.Path
	if strings.HasSuffix(strings.ToLower(pkg.Path), ".msi") {
		exe, err = msiexecPath()
		if err != nil {
			return Result{}, err
		}
	}
	code, err := run(ctx, exe, args, filepath.Dir(pkg.Path))
	if err != nil {
		return Result{}, err
	}
	return interpretExit(code)
}
