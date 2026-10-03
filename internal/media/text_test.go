package media

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortAppName(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"exe", "Spotify.exe", "Spotify"},
		{"upper exe", "Spotify.EXE", "Spotify"},
		{"packaged app id", "SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify", "Spotify"},
		{"packaged dotted app id", "Microsoft.ZuneMusic_8wekyb3d8bbwe!Microsoft.ZuneMusic", "ZuneMusic"},
		{"packaged generic app id", "AppleInc.AppleMusic_nzyj5cx40ttqa!App", "AppleMusic"},
		{"package family only", "Microsoft.ZuneMusic_8wekyb3d8bbwe", "ZuneMusic"},
		{"plain name", "Chrome", "Chrome"},
		{"lower case exe", "chrome.exe", "Chrome"},
		{"dotted name outside a package", "Yandex.Music", "Yandex.Music"},
		{"trailing bang", "Foo.Bar_zpdnekdrzrea0!", "Bar"},
		{"only a bang", "!", ""},
		{"blank", "   ", ""},
		{"empty", "", ""},
		{"control characters", "Spot\x00ify.exe", "Spotify"},
		{"non latin", "яндекс.exe", "Яндекс"},
		{"too long", strings.Repeat("a", 150) + ".exe", "A" + strings.Repeat("a", maxAppName-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortAppName(tt.id); got != tt.want {
				t.Fatalf("shortAppName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"plain", "Song", 200, "Song"},
		{"empty", "", 200, ""},
		{"trims spaces", "  Song  ", 200, "Song"},
		{"null and bell dropped", "So\x00ng\x07", 200, "Song"},
		{"newline and tab become spaces", "A\nB\tC\rD", 200, "A B C D"},
		{"escape sequence dropped", "\x1b[31mred", 200, "[31mred"},
		{"c1 control dropped", "a\u0085b", 200, "ab"},
		{"bidi override dropped", "abc\u202edef\u2066g\u200fh", 200, "abcdefgh"},
		{"invalid utf8 dropped", "ab\xffcd", 200, "abcd"},
		{"cut by runes not bytes", strings.Repeat("я", 300), 200, strings.Repeat("я", 200)},
		{"cut exactly", "abcdef", 3, "abc"},
		{"cut leaves no trailing space", "abc def", 4, "abc"},
		{"controls do not count towards the limit", "\x00\x00\x00abc", 3, "abc"},
		{"emoji kept", "Mix 🎵", 200, "Mix 🎵"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clean(tt.in, tt.limit)
			if got != tt.want {
				t.Fatalf("clean(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("clean(%q, %d) produced invalid UTF-8: %q", tt.in, tt.limit, got)
			}
		})
	}
}
