package redact

import (
	"strings"
	"testing"
)

func TestCodexReviewQuotedSecret(t *testing.T) {
	out, err := Sanitize(`remote error: {"accessToken":"example-private-value"}`)
	if err == nil && strings.Contains(out, "example-private-value") {
		t.Fatalf("secret accepted unchanged: %s", out)
	}
}
