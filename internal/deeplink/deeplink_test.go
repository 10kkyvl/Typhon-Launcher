package deeplink

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAccepts(t *testing.T) {
	cases := []struct {
		name, raw string
		want      Target
	}{
		{"steam", "typhon://game?steam=440", Target{ProviderSteam, "440"}},
		{"igdb", "typhon://game?igdb=1942", Target{ProviderIGDB, "1942"}},
		{"browser slash", "typhon://game/?steam=1", Target{ProviderSteam, "1"}},
		{"scheme case", "TYPHON://game?igdb=7", Target{ProviderIGDB, "7"}},
		{"longest id", "typhon://game?steam=999999999999", Target{ProviderSteam, "999999999999"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if err != nil || got != tc.want {
				t.Fatalf("Parse(%q) = %+v, %v; want %+v", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, raw string
		want      error
	}{
		{"empty", "", ErrEmpty},
		{"too long", "typhon://game?steam=1&x=" + strings.Repeat("a", maxLinkLen), ErrTooLong},
		{"control character", "typhon://game?steam=1\n", ErrMalformed},
		{"backslash host", `typhon://game\..\?steam=1`, ErrMalformed},
		{"quote in host", `typhon://game"?steam=1`, ErrHost},
		{"other scheme", "https://game?steam=1", ErrScheme},
		{"no scheme", "game?steam=1", ErrScheme},
		{"steam scheme", "steam://rungameid/440", ErrScheme},
		{"opaque", "typhon:game?steam=1", ErrHost},
		{"other host", "typhon://install?steam=1", ErrHost},
		{"host case", "typhon://GAME?steam=1", ErrHost},
		{"port", "typhon://game:80?steam=1", ErrHost},
		{"no host", "typhon:///game?steam=1", ErrHost},
		{"userinfo", "typhon://user:pass@game?steam=1", ErrUserinfo},
		{"path", "typhon://game/play?steam=1", ErrPath},
		{"traversal path", "typhon://game/../install?steam=1", ErrPath},
		{"double slash", "typhon://game//?steam=1", ErrPath},
		{"fragment", "typhon://game?steam=1#x", ErrFragment},
		{"empty fragment", "typhon://game?steam=1#", ErrFragment},
		{"no query", "typhon://game", ErrParams},
		{"empty query", "typhon://game?", ErrParams},
		{"no value", "typhon://game?steam", ErrParams},
		{"no key", "typhon://game?=1", ErrParams},
		{"two params", "typhon://game?steam=1&igdb=2", ErrParams},
		{"repeated param", "typhon://game?steam=1&steam=2", ErrParams},
		{"extra param", "typhon://game?steam=1&autoinstall=1", ErrParams},
		{"semicolon", "typhon://game?steam=1;igdb=2", ErrParams},
		{"second equals", "typhon://game?steam=1=2", ErrParams},
		{"unknown provider", "typhon://game?gog=1", ErrProvider},
		{"provider case", "typhon://game?Steam=1", ErrProvider},
		{"encoded key", "typhon://game?st%65am=1", ErrProvider},
		{"empty id", "typhon://game?steam=", ErrID},
		{"letters", "typhon://game?steam=abc", ErrID},
		{"negative", "typhon://game?steam=-1", ErrID},
		{"plus", "typhon://game?steam=+1", ErrID},
		{"space", "typhon://game?steam=1%20", ErrID},
		{"encoded digit", "typhon://game?steam=%31", ErrID},
		{"traversal id", "typhon://game?steam=../1", ErrID},
		{"quote injection", `typhon://game?steam=1"--play`, ErrID},
		{"argument injection", "typhon://game?steam=1 --play abc", ErrID},
		{"play flag", "typhon://game?steam=--play", ErrID},
		{"unicode digits", "typhon://game?steam=٣", ErrID},
		{"thirteen digits", "typhon://game?steam=1234567890123", ErrID},
		{"leading zero", "typhon://game?steam=01", ErrID},
		{"zero", "typhon://game?steam=0", ErrZeroID},
		{"zeros", "typhon://game?igdb=0000", ErrZeroID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse(%q) error = %v, want %v", tc.raw, err, tc.want)
			}
			if got != (Target{}) {
				t.Fatalf("Parse(%q) = %+v alongside an error", tc.raw, got)
			}
		})
	}
}

func TestParseErrorDoesNotQuoteTheLink(t *testing.T) {
	_, err := Parse("typhon://game?steam=1\nsecret-payload")
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-payload") || err.Error() != ErrMalformed.Error() {
		t.Fatalf("error text carries the input: %q", err)
	}
}

func TestIsLink(t *testing.T) {
	for arg, want := range map[string]bool{
		"typhon://game?steam=1": true,
		"TYPHON:x":              true,
		"typhon:":               true,
		"typhon":                false,
		"--play":                false,
		"":                      false,
		"xtyphon:":              false,
	} {
		if got := IsLink(arg); got != want {
			t.Errorf("IsLink(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestTargetProviderID(t *testing.T) {
	if got := (Target{ProviderSteam, "440"}).providerID(); got != "steam:440" {
		t.Fatalf("steam = %q", got)
	}
	if got := (Target{ProviderIGDB, "7"}).providerID(); got != "7" {
		t.Fatalf("igdb = %q", got)
	}
}
