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
			if err != nil || got != (Link{Target: tc.want}) {
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
			if got != (Link{}) {
				t.Fatalf("Parse(%q) = %+v alongside an error", tc.raw, got)
			}
		})
	}
}

func TestParseSearchAccepts(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"latin", "typhon://search?q=Hades", "Hades"},
		{"browser slash", "typhon://search/?q=Hades", "Hades"},
		{"scheme case", "TYPHON://search?q=Hades", "Hades"},
		{"cyrillic", "typhon://search?q=%D0%92%D0%B5%D0%B4%D1%8C%D0%BC%D0%B0%D0%BA%203", "Ведьмак 3"},
		{"emoji", "typhon://search?q=%F0%9F%98%80", "😀"},
		{"escaped space", "typhon://search?q=Half%20Life", "Half Life"},
		{"plus stays a plus", "typhon://search?q=C++", "C++"},
		{"escaped plus", "typhon://search?q=C%2B%2B", "C++"},
		{"traversal is text", "typhon://search?q=../..", "../.."},
		{"escaped traversal is text", "typhon://search?q=%2E%2E%2Fetc", "../etc"},
		{"escaped quote", "typhon://search?q=Baldur%27s%20%22Gate%22", `Baldur's "Gate"`},
		{"markup is text", "typhon://search?q=%3Cb%3Ex%3C%2Fb%3E", "<b>x</b>"},
		{"play flag is text", "typhon://search?q=--play", "--play"},
		{"quote and play flag are text", "typhon://search?q=x%22%20--play%20g1", `x" --play g1`},
		{"escaped separators are text", "typhon://search?q=a%26b%3Dc%3Bd%23e", "a&b=c;d#e"},
		{"spaces trimmed", "typhon://search?q=%20%20Hades%20", "Hades"},
		{"longest", "typhon://search?q=" + strings.Repeat("a", maxQueryRunes), strings.Repeat("a", maxQueryRunes)},
		{"longest cyrillic", "typhon://search?q=" + strings.Repeat("%D1%8F", maxQueryRunes), strings.Repeat("я", maxQueryRunes)},
		{"longest possible link", "typhon://search/?q=" + strings.Repeat("%F0%9F%98%80", maxQueryRunes), strings.Repeat("😀", maxQueryRunes)},
		{"spaces do not count", "typhon://search?q=%20" + strings.Repeat("a", maxQueryRunes) + "%20", strings.Repeat("a", maxQueryRunes)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if err != nil || got != (Link{Query: tc.want}) {
				t.Fatalf("Parse(%q) = %+v, %v; want query %q", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestParseSearchRejects(t *testing.T) {
	cases := []struct {
		name, raw string
		want      error
	}{
		{"link over the limit", "typhon://search/?q=" + strings.Repeat("%F0%9F%98%80", maxQueryRunes) + "a", ErrTooLong},
		{"control character in raw link", "typhon://search?q=a\nb", ErrMalformed},
		{"host case", "typhon://SEARCH?q=a", ErrHost},
		{"port", "typhon://search:80?q=a", ErrHost},
		{"opaque", "typhon:search?q=a", ErrHost},
		{"userinfo", "typhon://user@search?q=a", ErrUserinfo},
		{"path", "typhon://search/play?q=a", ErrPath},
		{"fragment", "typhon://search?q=a#b", ErrFragment},
		{"no query", "typhon://search", ErrSearchParams},
		{"empty query", "typhon://search?", ErrSearchParams},
		{"no value", "typhon://search?q", ErrSearchParams},
		{"no key", "typhon://search?=a", ErrSearchParams},
		{"other key", "typhon://search?steam=1", ErrSearchParams},
		{"key case", "typhon://search?Q=a", ErrSearchParams},
		{"encoded key", "typhon://search?%71=a", ErrSearchParams},
		{"repeated q", "typhon://search?q=a&q=b", ErrSearchParams},
		{"extra param", "typhon://search?q=a&autoinstall=1", ErrSearchParams},
		{"param before q", "typhon://search?x=1&q=a", ErrSearchParams},
		{"semicolon", "typhon://search?q=a;x=1", ErrSearchParams},
		{"second equals", "typhon://search?q=a=b", ErrSearchParams},
		{"bad escape", "typhon://search?q=%ZZ", ErrQueryEscape},
		{"cut escape", "typhon://search?q=a%2", ErrQueryEscape},
		{"lone percent", "typhon://search?q=100%", ErrQueryEscape},
		{"raw quote", `typhon://search?q=a"b`, ErrQueryEscape},
		{"raw space", "typhon://search?q=a b", ErrQueryEscape},
		{"raw backslash", `typhon://search?q=a\b`, ErrQueryEscape},
		{"raw cyrillic", "typhon://search?q=Ведьмак", ErrQueryEscape},
		{"not utf-8", "typhon://search?q=%FF", ErrQueryUTF8},
		{"cut utf-8", "typhon://search?q=%D0", ErrQueryUTF8},
		{"overlong utf-8", "typhon://search?q=%C0%AE", ErrQueryUTF8},
		{"surrogate", "typhon://search?q=%ED%A0%80", ErrQueryUTF8},
		{"nul", "typhon://search?q=a%00b", ErrQueryControl},
		{"line feed", "typhon://search?q=a%0Ab", ErrQueryControl},
		{"carriage return", "typhon://search?q=a%0Db", ErrQueryControl},
		{"leading line feed", "typhon://search?q=%0Aab", ErrQueryControl},
		{"trailing line feed", "typhon://search?q=ab%0A", ErrQueryControl},
		{"tab", "typhon://search?q=a%09b", ErrQueryControl},
		{"delete", "typhon://search?q=a%7Fb", ErrQueryControl},
		{"c1 control", "typhon://search?q=a%C2%85b", ErrQueryControl},
		{"line separator", "typhon://search?q=a%E2%80%A8b", ErrQueryControl},
		{"paragraph separator", "typhon://search?q=a%E2%80%A9b", ErrQueryControl},
		{"right to left override", "typhon://search?q=a%E2%80%AEb", ErrQueryControl},
		{"empty value", "typhon://search?q=", ErrQueryEmpty},
		{"only spaces", "typhon://search?q=%20%20", ErrQueryEmpty},
		{"only wide spaces", "typhon://search?q=%E3%80%80", ErrQueryEmpty},
		{"121 runes", "typhon://search?q=" + strings.Repeat("a", maxQueryRunes+1), ErrQueryTooLong},
		{"121 cyrillic runes", "typhon://search?q=" + strings.Repeat("%D1%8F", maxQueryRunes+1), ErrQueryTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse(%q) error = %v, want %v", tc.raw, err, tc.want)
			}
			if got != (Link{}) {
				t.Fatalf("Parse(%q) = %+v alongside an error", tc.raw, got)
			}
		})
	}
}

func TestParseKeepsGameLinksStrict(t *testing.T) {
	longGame := "typhon://game?steam=1&x=" + strings.Repeat("a", maxLinkLen-len("typhon://game?steam=1&x=")+1)
	cases := []struct {
		name, raw string
		want      error
	}{
		{"game link over its own limit", longGame, ErrTooLong},
		{"escape in id", "typhon://game?steam=%31", ErrID},
		{"query key on a game link", "typhon://game?q=Hades", ErrProvider},
		{"id on a search link", "typhon://search?steam=440", ErrSearchParams},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.raw); !errors.Is(err, tc.want) {
				t.Fatalf("Parse(%q) error = %v, want %v", tc.raw, err, tc.want)
			}
		})
	}
}

func TestParseSearchErrorDoesNotQuoteTheLink(t *testing.T) {
	for _, raw := range []string{
		"typhon://search?q=secret-payload%ZZ",
		"typhon://search?q=secret-payload%FF",
		"typhon://search?q=secret-payload%0A",
		"typhon://search?q=secret-payload\"",
		"typhon://search?q=secret-payload&x=1",
	} {
		_, err := Parse(raw)
		if err == nil {
			t.Fatalf("Parse(%q) accepted", raw)
		}
		if strings.Contains(err.Error(), "secret-payload") {
			t.Fatalf("error text carries the input: %q", err)
		}
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
