package deeplink

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestErrorCodesMatchTheFrontendTable(t *testing.T) {
	goBody, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	tsBody, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "sources", "sourceErrors.ts"))
	if err != nil {
		t.Fatalf("read sourceErrors.ts: %v", err)
	}
	goCodes := regexp.MustCompile(`"(link\.[a-z_]+)"`).FindAllSubmatch(goBody, -1)
	if len(goCodes) < 5 {
		t.Fatalf("found %d link codes in service.go, want at least 5", len(goCodes))
	}
	for _, match := range goCodes {
		code := string(match[1])
		if !regexp.MustCompile(`'` + regexp.QuoteMeta(code) + `':`).Match(tsBody) {
			t.Errorf("%q is returned from Go but sourceErrors.ts has no text for it", code)
		}
	}
}
