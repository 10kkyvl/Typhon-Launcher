//go:build release

package main

import (
	"os"
	"strings"
	"testing"

	"typhon/internal/legal"
)

func TestReleaseReady(t *testing.T) {
	if err := legal.Validate(legalDocs); err != nil {
		t.Fatalf("legal.Validate: required legal document missing or empty: %v", err)
	}

	for _, name := range []string{"COPYRIGHT.md", "COPYRIGHT.en.md"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(data)

		if strings.Contains(body, "{{ABUSE_CONTACT}}") {
			t.Errorf("%s still contains the {{ABUSE_CONTACT}} placeholder; fill in the real abuse contact before release", name)
		}
		if strings.Contains(body, "TYPHON-RELEASE-BLOCKER") {
			t.Errorf("%s still contains a TYPHON-RELEASE-BLOCKER marker; resolve it before release", name)
		}
	}
}
