package theme

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	goCodePattern = regexp.MustCompile(`uierr\.(?:New|Wrap)\(\s*"([^"]+)"`)
	tsCodePattern = regexp.MustCompile(`'(theme\.[a-z0-9_]+)':`)
)

func sourceFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e, "_test.go") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func codesIn(t *testing.T, pattern *regexp.Regexp, paths ...string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("не прочитан %s: %v", path, err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
			seen[match[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Коды ошибок — контракт между Go и интерфейсом: переименование с одной
// стороны не ломает сборку, а тихо возвращает пользователю запасной текст.
func TestErrorCodesMatchTheFrontendTable(t *testing.T) {
	goCodes := codesIn(t, goCodePattern, sourceFiles(t, ".")...)
	if len(goCodes) < 12 {
		t.Fatalf("в пакете найдено %d кодов, ожидалось не меньше 12", len(goCodes))
	}

	tsPath := filepath.Join("..", "..", "frontend", "src", "lib", "theme", "themeErrors.ts")
	tsCodes := codesIn(t, tsCodePattern, tsPath)

	inTS := map[string]bool{}
	for _, code := range tsCodes {
		inTS[code] = true
	}
	for _, code := range goCodes {
		if !inTS[code] {
			t.Errorf("код %q возвращается из Go, но не переводится в themeErrors.ts", code)
		}
	}

	inGo := map[string]bool{}
	for _, code := range goCodes {
		inGo[code] = true
	}
	for _, code := range tsCodes {
		if !inGo[code] {
			t.Errorf("код %q переводится в themeErrors.ts, но Go его не возвращает", code)
		}
	}
}
