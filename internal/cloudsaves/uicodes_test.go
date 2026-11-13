package cloudsaves

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	goCodePattern = regexp.MustCompile(`"(cloud\.[a-z0-9_]+)"`)
	tsCodePattern = regexp.MustCompile(`'(cloud\.[a-z0-9_]+)':`)
)

func codesIn(t *testing.T, pattern *regexp.Regexp, paths ...string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
			seen[match[1]] = true
		}
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}

func packageSources(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var out []string
	for _, p := range paths {
		if !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
	}
	return out
}

// The cloud.* codes reach the player as text looked up by code: a code
// without a key shows the fallback, a key without a code is dead text.
func TestErrorCodesMatchTheFrontendCatalogs(t *testing.T) {
	goCodes := codesIn(t, goCodePattern, packageSources(t)...)
	if len(goCodes) < 35 {
		t.Fatalf("found %d cloud.* codes in the package, want at least 35: the scan is broken", len(goCodes))
	}
	inGo := map[string]bool{}
	for _, code := range goCodes {
		inGo[code] = true
	}
	for _, lang := range []string{"ru", "en"} {
		path := filepath.Join("..", "..", "frontend", "src", "lib", "i18n", "catalog", lang, "cloudsaves.ts")
		keys := codesIn(t, tsCodePattern, path)
		inTS := map[string]bool{}
		for _, key := range keys {
			inTS[key] = true
		}
		for _, code := range goCodes {
			if !inTS[code] {
				t.Errorf("%s: code %q is returned by cloudsaves but has no text", lang, code)
			}
		}
		for _, key := range keys {
			if !inGo[key] {
				t.Errorf("%s: key %q has a text but cloudsaves never returns it", lang, key)
			}
		}
	}
}
