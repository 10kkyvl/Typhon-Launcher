package theme

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var rootTokenRe = regexp.MustCompile(`(?m)^\s*(--[a-zA-Z0-9-]+)\s*:`)

func TestAllowedTokensMatchStylesheet(t *testing.T) {
	data, err := os.ReadFile("../../frontend/src/styles/tokens.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	content := string(data)
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end < 0 || end < start {
		t.Fatalf("no :root block found in stylesheet")
	}
	block := content[start+1 : end]

	owned := map[string]bool{}
	for _, name := range settingsOwnedTokens {
		owned[name] = true
	}

	got := map[string]bool{}
	for _, m := range rootTokenRe.FindAllStringSubmatch(block, -1) {
		if owned[m[1]] {
			continue
		}
		got[m[1]] = true
	}

	want := map[string]bool{}
	for _, tok := range allowedTokens {
		want[tok.Name] = true
	}

	for name := range got {
		if !want[name] {
			t.Errorf("stylesheet defines %s but allowedTokens does not", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("allowedTokens defines %s but stylesheet does not", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("token count mismatch: stylesheet=%d allowedTokens=%d", len(got), len(want))
	}
}
