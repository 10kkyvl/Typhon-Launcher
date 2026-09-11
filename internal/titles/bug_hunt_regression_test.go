package titles

import "testing"

func TestRomanVIsNotParsedAsVersion(t *testing.T) {
	for _, input := range []string{
		"Grand Theft Auto V 1.0.2845",
		"Civilization V 1.0",
		"Battlefield V 1.2",
	} {
		parsed := Parse(input)
		if parsed.Base == "Grand Theft Auto" || parsed.Base == "Civilization" || parsed.Base == "Battlefield" {
			t.Fatalf("Parse(%q) dropped Roman V: base=%q version=%q", input, parsed.Base, parsed.Version)
		}
	}
	if parsed := Parse("112 Operator v 0.250801"); parsed.Version != "0.250801" {
		t.Fatalf("lowercase spaced version = %q, want 0.250801", parsed.Version)
	}
}
