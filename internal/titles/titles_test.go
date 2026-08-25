package titles

import (
	"strings"
	"testing"
)

func TestParsePositive(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		check func(t *testing.T, p Parsed)
	}{
		{
			name: "cyberpunk ultimate",
			raw:  "Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19.x64",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Cyberpunk 2077")
				want(t, "Edition", p.Edition, "Ultimate Edition")
				want(t, "Version", p.Version, "2.31")
				mustContainStr(t, "Languages", p.Languages, "MULTi19")
				mustContainStr(t, "Tags", p.Tags, "x64")
			},
		},
		{
			name: "witcher complete",
			raw:  "The.Witcher.3.Wild.Hunt.Complete.Edition.v4.04",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "The Witcher 3 Wild Hunt")
				want(t, "Edition", p.Edition, "Complete Edition")
				want(t, "Version", p.Version, "4.04")
			},
		},
		{
			name: "doom deluxe update",
			raw:  "DOOM.Eternal.Deluxe.Edition.Update.9",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "DOOM Eternal")
				want(t, "Edition", p.Edition, "Deluxe Edition")
				want(t, "Version", p.Version, "9")
				if !strings.Contains(p.RawVersion, "Update") {
					t.Errorf("RawVersion %q does not contain Update", p.RawVersion)
				}
			},
		},
		{
			name: "rdr2 build",
			raw:  "Red.Dead.Redemption.2.Build.1491.50",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Red Dead Redemption 2")
				want(t, "Version", p.Version, "1491.50")
			},
		},
		{
			name: "baldurs patch",
			raw:  "Baldurs.Gate.3.Patch.8",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Baldurs Gate 3")
				want(t, "Version", p.Version, "8")
			},
		},
		{
			name: "cyberpunk spaces",
			raw:  "Cyberpunk 2077 MULTi19 v2.31",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Cyberpunk 2077")
			},
		},
		{
			name: "hogwarts fitgirl",
			raw:  "Hogwarts Legacy Digital Deluxe Edition [FitGirl Repack]",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Hogwarts Legacy")
				mustContainStr(t, "Tags", p.Tags, "repack")
			},
		},
		{
			name: "prey year",
			raw:  "Prey (2017) [MULTi9] v1.0",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Prey")
				if p.Year != 2017 {
					t.Errorf("Year = %d, want 2017", p.Year)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, Parse(tc.raw))
		})
	}
}

func TestParseNegative(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantBase   string
		wantNoEdit bool
	}{
		{"ultimate chicken horse", "Ultimate Chicken Horse", "Ultimate Chicken Horse", true},
		{"game of the year", "Game of the Year", "Game of the Year", true},
		{"deluxe ski jump", "Deluxe Ski Jump 4", "Deluxe Ski Jump 4", true},
		{"need for speed", "Need for Speed Most Wanted", "Need for Speed Most Wanted", true},
		{"dirt rally dotted version", "DiRT Rally 2.0", "DiRT Rally 2.0", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Parse(tc.raw)
			want(t, "Base", p.Base, tc.wantBase)
			if tc.wantNoEdit && p.Edition != "" {
				t.Errorf("Edition = %q, want empty", p.Edition)
			}
		})
	}
}

func TestParseNeverEmptiesBase(t *testing.T) {
	inputs := []string{
		"Ultimate Chicken Horse",
		"Game of the Year",
		"Deluxe Ski Jump 4",
		"Need for Speed Most Wanted",
		"DiRT Rally 2.0",
		"Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19.x64",
		"The.Witcher.3.Wild.Hunt.Complete.Edition.v4.04",
		"DOOM.Eternal.Deluxe.Edition.Update.9",
		"Red.Dead.Redemption.2.Build.1491.50",
		"Baldurs.Gate.3.Patch.8",
		"Cyberpunk 2077 MULTi19 v2.31",
		"Hogwarts Legacy Digital Deluxe Edition [FitGirl Repack]",
		"Prey (2017) [MULTi9] v1.0",
		"Half-Life 2",
		"Portal.2.Update.9.RUS.ENG",
		"Stardew Valley GOG",
		"Elden Ring Deluxe Edition [Steam-Rip]",
		"Grand Theft Auto V Premium Edition v1.0.2372.0",
		"It Takes Two CODEX",
		"A Plague Tale Requiem MULTi14 Repack by CODEX",
	}

	for _, raw := range inputs {
		p := Parse(raw)
		if p.Base == "" {
			t.Errorf("Parse(%q).Base is empty", raw)
		}
	}

	if got := Parse("   ").Base; got != "" {
		t.Errorf("Parse of whitespace-only input should have empty Base, got %q", got)
	}
}

func TestNormalizeMatchesApostrophe(t *testing.T) {
	a := Normalize("Baldur's Gate 3")
	b := Parse("Baldurs.Gate.3.Patch.8").Normalized
	if a != b {
		t.Errorf("Normalize(%q) = %q, Parse(...).Normalized = %q, want equal", "Baldur's Gate 3", a, b)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Baldur's Gate 3", "baldurs gate 3"},
		{"Baldurs Gate 3", "baldurs gate 3"},
		{"Café Society", "cafe society"},
		{"Cats & Dogs", "cats and dogs"},
		{"The Witcher 3: Wild Hunt", "the witcher 3 wild hunt"},
		{"  Multiple   Spaces  ", "multiple spaces"},
	}

	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParsedNormalizedMatchesNormalizeOfBase(t *testing.T) {
	inputs := []string{
		"Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19.x64",
		"Ultimate Chicken Horse",
		"Prey (2017) [MULTi9] v1.0",
	}
	for _, raw := range inputs {
		p := Parse(raw)
		if p.Normalized != Normalize(p.Base) {
			t.Errorf("Parse(%q).Normalized = %q, Normalize(Base) = %q", raw, p.Normalized, Normalize(p.Base))
		}
	}
}

func want(t *testing.T, field, got, expected string) {
	t.Helper()
	if got != expected {
		t.Errorf("%s = %q, want %q", field, got, expected)
	}
}

func mustContainStr(t *testing.T, field string, list []string, val string) {
	t.Helper()
	for _, v := range list {
		if v == val {
			return
		}
	}
	t.Errorf("%s = %v, want to contain %q", field, list, val)
}
