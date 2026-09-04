package selfupdate

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	goCodes := selfupdateCodesIn(t, selfupdateGoCodePattern,
		"errors.go", "paths.go", "client.go", "download.go",
		"apply.go", "apply_installer.go", "apply_windows.go", "apply_devmock.go",
		"worker.go", "worker_windows.go", "worker_devmock.go", "service.go")
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
