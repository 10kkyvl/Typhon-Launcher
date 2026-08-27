package redact

import (
	"strings"
	"testing"
)

// The frontend scrubs before calling into Go and Go scrubs again. The second
// pass must not be defeated by the first pass's placeholders: an earlier
// frontend rule left "C:\Users\<user>\Typhon\state.json", and Go's patterns
// stop at '<', so the readable tail survived and Sanitize reported it clean.
func TestInteropWithFrontendSanitizer(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"windows", `open <path>: denied`},
		{"unix", `open <path>: denied`},
		{"url kept", `fetch https://api.typhon.app/v1/download failed`},
		{"stack", "at run (<path>:10:5)\n at boot (<path>:2:1)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Sanitize(c.in)
			if err != nil {
				t.Fatalf("Sanitize(%q) refused frontend-scrubbed text: %v", c.in, err)
			}
			for _, leak := range []string{`C:\`, "/home/", "/Users/", "<user>"} {
				if strings.Contains(out, leak) {
					t.Fatalf("Sanitize(%q) = %q still carries %q", c.in, out, leak)
				}
			}
		})
	}
}
