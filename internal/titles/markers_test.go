package titles

import (
	"strings"
	"testing"
)

func TestParseMarkers(t *testing.T) {
	cases := []struct {
		raw      string
		base     string
		tags     []string
		repacker string
	}{
		{
			raw:  "9-Bit Armies: A Bit Too Far — v864547 (Build 19702616) | Архив",
			tags: []string{"archive"},
		},
		{
			raw:  "1348 Ex Voto — (Build 23638420) | Portable",
			base: "1348 Ex Voto",
			tags: []string{"portable"},
		},
		{
			raw:  "Cursed Feed — (Build 16951281) | P2P",
			base: "Cursed Feed",
			tags: []string{"p2p"},
		},
		{
			raw:  "Blue Wednesday — v1.0 (69123) | GOG",
			tags: []string{"gog"},
		},
		{
			raw:      "1979 Revolution: Black Friday — RePack от R.G. Механики",
			base:     "1979 Revolution: Black Friday",
			tags:     []string{"repack", "mechanics"},
			repacker: "mechanics",
		},
		{
			raw:      "007 First Light — RePack от Igruha",
			base:     "007 First Light",
			tags:     []string{"repack", "igruha"},
			repacker: "igruha",
		},
		{
			raw:      "Grand Theft Auto V — Repack by MOP030B от Zlofenix",
			base:     "Grand Theft Auto V",
			tags:     []string{"repack"},
			repacker: "zlofenix",
		},
		{
			raw:  "101 Ways To Die — RePack",
			base: "101 Ways To Die",
			tags: []string{"repack"},
		},
		{
			raw:      "Deep Rock Galactic — Steam-Rip от Chovka",
			base:     "Deep Rock Galactic",
			tags:     []string{"steam-rip", "chovka"},
			repacker: "chovka",
		},
		{
			raw:  "Artemis — v0.5.1 | Лицензия",
			base: "Artemis",
			tags: []string{"license"},
		},
		{
			raw:  "Element TD 2 — Ранний доступ",
			base: "Element TD 2",
			tags: []string{"early-access"},
		},
		{
			raw:  "Monster Energy Supercross The Official Videogame — CODEX",
			base: "Monster Energy Supercross The Official Videogame",
			tags: []string{"p2p"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := Parse(tc.raw)
			if tc.base != "" {
				want(t, "Base", p.Base, tc.base)
			}
			for _, tag := range tc.tags {
				mustContainStr(t, "Tags", p.Tags, tag)
			}
			want(t, "Repacker", Repacker(p.Tags), tc.repacker)
		})
	}
}

// Маркер опознаётся только целиком: иначе «| Season 1» и «Portable» из названия
// игры уехали бы в теги, а часть заголовка потерялась бы.
func TestParseMarkersKeepTitleParts(t *testing.T) {
	cases := []struct {
		raw     string
		base    string
		noTag   string
		version string
	}{
		{raw: "Persona 3 Portable", base: "Persona 3 Portable", noTag: "portable"},
		{
			raw:     "A Rat's Quest - The Way Back Home | Season 1 — v26-02-11 (90160) | GOG",
			noTag:   "archive",
			version: "26",
		},
		{raw: "Cantata — v1.0.2", base: "Cantata", noTag: "archive", version: "1.0.2"},
		{raw: "March of the Eagles", base: "March of the Eagles", noTag: "p2p"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := Parse(tc.raw)
			if tc.base != "" {
				want(t, "Base", p.Base, tc.base)
			}
			if tc.version != "" {
				want(t, "Version", p.Version, tc.version)
			}
			for _, tag := range p.Tags {
				if tag == tc.noTag {
					t.Errorf("Tags = %v, не должно содержать %q", p.Tags, tc.noTag)
				}
			}
		})
	}
}

func TestParseMarkersKeepSeasonInTitle(t *testing.T) {
	p := Parse("A Rat's Quest - The Way Back Home | Season 1 — v26-02-11 (90160) | GOG")
	mustContainStr(t, "Tags", p.Tags, "gog")
	if !strings.Contains(p.Base, "Season 1") {
		t.Errorf("Base = %q, «Season 1» не должно было пропасть из названия", p.Base)
	}
}

func TestParseServiceSuffixes(t *testing.T) {
	cases := []struct {
		raw     string
		base    string
		version string
		tags    []string
		langs   []string
	}{
		{
			raw:  "112 Operator [Папка игры] PC | Лицензия",
			base: "112 Operator",
			tags: []string{"license", "pc", "portable"},
		},
		{
			raw:     "112 Operator v 0.250801 [Архив]",
			base:    "112 Operator",
			version: "0.250801",
			tags:    []string{"archive"},
		},
		{
			raw:  "11F (+ Windows 7 Fix, )",
			base: "11F",
			tags: []string{"windows-7-fix"},
		},
		{
			raw:   "11F (+ Windows 7 Fix, MULTi6) [FitGirl Repack]",
			base:  "11F",
			tags:  []string{"fitgirl", "repack", "windows-7-fix"},
			langs: []string{"MULTi6"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := Parse(tc.raw)
			want(t, "Base", p.Base, tc.base)
			want(t, "Version", p.Version, tc.version)
			for _, tag := range tc.tags {
				mustContainStr(t, "Tags", p.Tags, tag)
			}
			for _, lang := range tc.langs {
				mustContainStr(t, "Languages", p.Languages, lang)
			}
		})
	}
}

func TestParseKeepsMeaningfulParentheses(t *testing.T) {
	cases := []struct {
		raw  string
		base string
	}{
		{"1000 Deaths (Thousand Deaths)", "1000 Deaths (Thousand Deaths)"},
		{"Resident Evil 4 (2023) Remake", "Resident Evil 4 Remake"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := Parse(tc.raw).Base; got != tc.base {
				t.Fatalf("Parse(%q).Base = %q, want %q", tc.raw, got, tc.base)
			}
		})
	}
}
