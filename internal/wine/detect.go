package wine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// defaultRoots перечисляет места, куда CrossOver ставится: системный
// /Applications и пользовательский ~/Applications.
func defaultRoots() []string {
	roots := []string{"/Applications/CrossOver.app"}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Applications", "CrossOver.app"))
	}
	return roots
}

// Detect ищет CrossOver в обычных местах установки.
func Detect() (Runtime, error) { return detectAt(defaultRoots()...) }

func detectAt(roots ...string) (Runtime, error) {
	for _, root := range roots {
		rt, err := runtimeAt(root)
		if err == nil {
			return rt, nil
		}
	}
	return Runtime{}, fmt.Errorf("%w: искали в %v", ErrNotInstalled, roots)
}

func runtimeAt(root string) (Runtime, error) {
	bin := filepath.Join(root, "Contents", "SharedSupport", "CrossOver", "bin")
	rt := Runtime{
		Root:       root,
		CxBottle:   filepath.Join(bin, "cxbottle"),
		CxStart:    filepath.Join(bin, "cxstart"),
		WineServer: filepath.Join(bin, "wineserver"),
	}
	for _, path := range []string{rt.CxBottle, rt.CxStart, rt.WineServer} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return Runtime{}, ErrNotInstalled
		}
	}
	rt.Version = bundleVersion(filepath.Join(root, "Contents", "Info.plist"))
	return rt, nil
}

// versionPattern читает CFBundleShortVersionString без парсера plist: версия
// нужна только для строки в «О программе» и диагностики, поэтому зависимость
// ради неё не оправдана, а её отсутствие не ошибка.
var versionPattern = regexp.MustCompile(
	`(?s)<key>CFBundleShortVersionString</key>\s*<string>([^<]*)</string>`)

func bundleVersion(plistPath string) string {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return ""
	}
	match := versionPattern.FindSubmatch(data)
	if match == nil {
		return ""
	}
	return string(match[1])
}
