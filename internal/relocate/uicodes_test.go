package relocate

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

var (
	goCodePattern = regexp.MustCompile(`uierr\.(?:New|Wrap)\(\s*"([^"]+)"`)
	tsCodePattern = regexp.MustCompile(`'(relocate\.[a-z0-9_]+)':`)
)

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
	goCodes := codesIn(t, goCodePattern, "model.go", "service.go")
	if len(goCodes) < 15 {
		t.Fatalf("в пакете найдено %d кодов, ожидалось не меньше 15", len(goCodes))
	}

	tsPath := filepath.Join("..", "..", "frontend", "src", "lib", "relocate", "moveMessages.ts")
	tsCodes := codesIn(t, tsCodePattern, tsPath)

	inTS := map[string]bool{}
	for _, code := range tsCodes {
		inTS[code] = true
	}
	for _, code := range goCodes {
		if !inTS[code] {
			t.Errorf("код %q возвращается из Go, но не переводится в moveMessages.ts", code)
		}
	}

	inGo := map[string]bool{}
	for _, code := range goCodes {
		inGo[code] = true
	}
	for _, code := range tsCodes {
		if !inGo[code] {
			t.Errorf("код %q переводится в moveMessages.ts, но Go его не возвращает", code)
		}
	}
}
