package titles

import (
	"strings"
	"testing"
)

func TestRomanVIsNotParsedAsVersion(t *testing.T) {
	for _, input := range []string{
		"Grand Theft Auto V 1.0.2845",
		"Civilization V 1.0",
		"Battlefield V 1.2",
		"grand theft auto v 1.0.2845",
		"civilization v 1.0",
		"battlefield v 1.2",
	} {
		parsed := Parse(input)
		if !strings.HasSuffix(strings.ToLower(parsed.Base), " v") {
			t.Fatalf("Parse(%q) dropped Roman V: base=%q version=%q", input, parsed.Base, parsed.Version)
		}
	}
	if parsed := Parse("112 Operator v 0.250801"); parsed.Version != "0.250801" {
		t.Fatalf("lowercase spaced version = %q, want 0.250801", parsed.Version)
	}
}

func TestVersionPatternRequiresNumericEvidence(t *testing.T) {
	for _, input := range []string{"V.O.I.D", "Vigil.The.Longest.Night", "VA-11 Hall-A", "V1РУЗ", "Vecter2", "Vex3"} {
		parsed := Parse(input)
		if parsed.Version != "" || parsed.Base == "" {
			t.Fatalf("Parse(%q) treated title text as version: base=%q version=%q", input, parsed.Base, parsed.Version)
		}
	}
}

func TestCompactLetterSuffixedVersionIsParsed(t *testing.T) {
	parsed := Parse("Grandma, No! Deluxe Edition – v20250522R + Bonus Content")
	if parsed.Version != "20250522R" || parsed.Base != "Grandma, No!" || parsed.Edition != "Deluxe Edition" {
		t.Fatalf("compact release version was not parsed without losing edition: %+v", parsed)
	}
}

func TestVersionContinuationKeepsPackagingAndDLCMarkers(t *testing.T) {
	if parsed := Parse("Example Game v1.2 + All"); parsed.Version != "1.2" || parsed.Base != "Example Game + All" {
		t.Fatalf("packaging text was consumed as version continuation: %+v", parsed)
	}
	if parsed := Parse("Example Game v1.2 + 123 DLCs"); parsed.Version != "1.2" || parsed.DLCCount != 123 || parsed.Base != "Example Game" {
		t.Fatalf("DLC marker was consumed as version continuation: %+v", parsed)
	}
}

func TestVersionContinuationDoesNotEatRepackerTags(t *testing.T) {
	for raw, tag := range map[string]string{
		"Example v1.0-GOG":   "gog",
		"Example v1.0-CODEX": "codex",
	} {
		parsed := Parse(raw)
		if parsed.Version != "1.0" || parsed.Base != "Example" {
			t.Fatalf("packaging suffix changed title/version: Parse(%q)=%+v", raw, parsed)
		}
		if len(parsed.Tags) != 1 || parsed.Tags[0] != tag {
			t.Fatalf("packaging suffix tag lost: Parse(%q)=%+v", raw, parsed)
		}
	}
}
