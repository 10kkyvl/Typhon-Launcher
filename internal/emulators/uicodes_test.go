package emulators

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var (
	goCodePattern = regexp.MustCompile(`uierr\.New\(\s*"(emulators\.[a-z0-9_]+)"`)
	tsCodePattern = regexp.MustCompile(`'(emulators\.[a-z0-9_]+)':`)
)

func codesIn(t *testing.T, pattern *regexp.Regexp, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	seen := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
		seen[match[1]] = true
	}
	return seen
}

func TestErrorCodesMatchTheFrontendTables(t *testing.T) {
	goCodes := codesIn(t, goCodePattern, "service.go")
	if len(goCodes) < 20 {
		t.Fatalf("found %d codes in service.go, want at least 20", len(goCodes))
	}
	for _, locale := range []string{"ru", "en"} {
		path := filepath.Join("..", "..", "frontend", "src", "lib", "i18n", "catalog", locale, "errEmulators.ts")
		tsCodes := codesIn(t, tsCodePattern, path)
		for code := range goCodes {
			if !tsCodes[code] {
				t.Errorf("%s: %q is returned from Go but has no text", locale, code)
			}
		}
		for code := range tsCodes {
			if !goCodes[code] {
				t.Errorf("%s: %q has a text but Go never returns it", locale, code)
			}
		}
	}
}
