package selfupdate

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	selfupdateGoCodePattern = regexp.MustCompile(`uierr\.(?:New|Wrap)\(\s*"([^"]+)"`)
	selfupdateTSCodePattern = regexp.MustCompile(`'(selfupdate\.[a-z0-9_]+)':`)
)

func selfupdateCodesIn(t *testing.T, pattern *regexp.Regexp, paths ...string) []string {
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
// Файлы читаются как текст, а не собираются, поэтому windows- и
// devmock-only варианты попадают в сравнение вместе с общими.
func TestErrorCodesMatchTheFrontendTable(t *testing.T) {
	// Список файлов раньше вёлся руками, и новый платформенный файл в него
	// просто не попадал: коды из него не проверялись вовсе. Берём весь пакет.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("не найдены файлы пакета: %v", err)
	}
	sources := make([]string, 0, len(files))
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		sources = append(sources, f)
	}
	goCodes := selfupdateCodesIn(t, selfupdateGoCodePattern, sources...)
	if len(goCodes) < 40 {
		t.Fatalf("в пакете найдено %d кодов, ожидалось не меньше 40", len(goCodes))
	}

	tsPath := filepath.Join("..", "..", "frontend", "src", "lib", "services", "selfupdateMessages.ts")
	tsCodes := selfupdateCodesIn(t, selfupdateTSCodePattern, tsPath)

	inTS := map[string]bool{}
	for _, code := range tsCodes {
		inTS[code] = true
	}
	for _, code := range goCodes {
		if !inTS[code] {
			t.Errorf("код %q возвращается из Go, но не переводится в selfupdateMessages.ts", code)
		}
	}

	inGo := map[string]bool{}
	for _, code := range goCodes {
		inGo[code] = true
	}
	for _, code := range tsCodes {
		if !inGo[code] {
			t.Errorf("код %q переводится в selfupdateMessages.ts, но Go его не возвращает", code)
		}
	}
}
