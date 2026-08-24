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

	data, err := os.ReadFile("COPYRIGHT.md")
	if err != nil {
		t.Fatalf("read COPYRIGHT.md: %v", err)
	}
	body := string(data)

	if strings.Contains(body, "{{ABUSE_CONTACT}}") {
		t.Error("COPYRIGHT.md still contains the {{ABUSE_CONTACT}} placeholder; fill in the real abuse contact before release")
	}
	if strings.Contains(body, "TYPHON-RELEASE-BLOCKER") {
		t.Error("COPYRIGHT.md still contains a TYPHON-RELEASE-BLOCKER marker; resolve it before release")
	}
}
