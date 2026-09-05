package library

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
	tsCodePattern = regexp.MustCompile(`'(library\.[a-z0-9_]+)':`)
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
	var libraryCodes []string
	for _, code := range goCodes {
		if strings.HasPrefix(code, "library.") {
			libraryCodes = append(libraryCodes, code)
		}
	}
	if len(libraryCodes) < 20 {
		t.Fatalf("в пакете найдено %d кодов library.*, ожидалось не меньше 20", len(libraryCodes))
	}

	tsPath := filepath.Join("..", "..", "frontend", "src", "lib", "i18n", "catalog", "ru", "errLibrary.ts")
	tsCodes := codesIn(t, tsCodePattern, tsPath)

	inTS := map[string]bool{}
	for _, code := range tsCodes {
		inTS[code] = true
	}
	for _, code := range libraryCodes {
		if !inTS[code] {
			t.Errorf("код %q возвращается из Go, но не переводится в errLibrary.ts", code)
		}
	}

	inGo := map[string]bool{}
	for _, code := range libraryCodes {
		inGo[code] = true
	}
	for _, code := range tsCodes {
		if !inGo[code] {
			t.Errorf("код %q переводится в errLibrary.ts, но library его не возвращает", code)
		}
	}
}
