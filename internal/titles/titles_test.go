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
			name: "dotted v prefix",
			raw:  "Hollow Knight: Silksong v.1.0.29315 [Папка игры] (2025)",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Hollow Knight: Silksong")
				want(t, "Version", p.Version, "1.0.29315")
				if p.Year != 2025 {
					t.Errorf("Year = %d, want 2025", p.Year)
				}
			},
		},
		{
			name: "portable after pipe",
			raw:  "Ex Voto — (Build 23638420) | Portable",
			check: func(t *testing.T, p Parsed) {
				want(t, "Normalized", p.Normalized, "ex voto")
				mustContainStr(t, "Tags", p.Tags, "portable")
			},
		},
		{
			name: "portable in brackets",
			raw:  "Celeste [Portable] (2019)",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Celeste")
				mustContainStr(t, "Tags", p.Tags, "portable")
			},
		},
		{
			name: "russian folder bracket",
			raw:  "Warhammer 40000 Space Marine 2 [Папка игры]",
			check: func(t *testing.T, p Parsed) {
				want(t, "Base", p.Base, "Warhammer 40000 Space Marine 2")
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
		{"persona portable", "Persona 3 Portable", "Persona 3 Portable", true},
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

func TestTitlesDLCCount(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"Cyberpunk 2077 + 5 DLCs", 5},
		{"Some Game +5 DLC", 5},
		{"Some Game + 12 DLC's", 12},
		{"Игра + 5 дополнений", 5},
		{"Cyberpunk 2077", 0},
	}
	for _, tc := range cases {
		p := Parse(tc.raw)
		if p.DLCCount != tc.want {
			t.Errorf("Parse(%q).DLCCount = %d, want %d", tc.raw, p.DLCCount, tc.want)
		}
		if strings.Contains(strings.ToLower(p.Base), "dlc") {
			t.Errorf("Parse(%q).Base = %q still contains DLC phrase", tc.raw, p.Base)
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

// Хвост версии, скобка сразу за версией и кириллический маркер репака — всё
// это метаданные раздачи. Пока они оставались в названии, серверный каталог не
// находил игру: он сверяет имена точным совпадением.
func TestReleaseMetadataLeavesTheTitle(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantBase    string
		wantVersion string
	}{
		{"буква в конце версии", "Within the Cosmos v.2.1.1a [Папка игры] (2025)", "Within the Cosmos", "2.1.1a"},
		{"буква в конце длинной версии", "Gas Station Simulator v.1.0.2.22714S [Папка игры] (2021)", "Gas Station Simulator", "1.0.2.22714S"},
		{"буква после минорной версии", "Way of the Hunter v.1.25h [GOG] (2022)", "Way of the Hunter", "1.25h"},
		{"подчёркивание в версии", "Little Kitty, Big City v.1.25.8.27_5409 [Архив] (2024)", "Little Kitty, Big City", "1.25.8.27_5409"},
		{"скобка сразу за версией", "Liminality – v1.0 (Release)", "Liminality", "1.0"},
		{"номер сборки в скобке за версией", "Astral Ascent – v2.4.0 (1181)", "Astral Ascent", "2.4.0"},
		{"кодовое имя в скобке за версией", "Crusader Kings III: Collection, v1.16.0 (Chamfron)", "Crusader Kings III: Collection", "1.16.0"},
		{"дата в скобке за версией", "Caribbean Legend: Complete Edition, v1.3.0 (11.09.24)", "Caribbean Legend", "1.3.0"},
		{"кириллический репак с именем", "Unravel (2016) PC | Репак от xatab", "Unravel", ""},
		{"репак без имени", "Frontline Zed (2019) RePack от", "Frontline Zed", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Parse(tc.raw)
			if p.Base != tc.wantBase {
				t.Errorf("Base = %q, want %q", p.Base, tc.wantBase)
			}
			if p.Version != tc.wantVersion {
				t.Errorf("Version = %q, want %q", p.Version, tc.wantVersion)
			}
		})
	}
}

// Год в скобке — единственное, что отличает тёзок, поэтому скобка за версией
// его не съедает.
func TestVersionDoesNotSwallowTheYear(t *testing.T) {
	p := Parse("Resident Evil 4 v1.0 (2005)")
	if p.Base != "Resident Evil 4" || p.Year != 2005 {
		t.Fatalf("Base = %q, Year = %d, want %q and 2005", p.Base, p.Year, "Resident Evil 4")
	}
}

// Код языка совпадает с обычными словами: Ara — это и арабский, и первое слово
// названия. Одиночный код — маркер только рядом с другими метаданными раздачи.
func TestLanguageCodeDoesNotEatTitleWords(t *testing.T) {
	for raw, want := range map[string]string{
		"Ara: History Untold v.2.0.2.528 [Папка игры] (2024)": "Ara: History Untold",
		"Ara: History Untold": "Ara: History Untold",
		"Spa Simulator":       "Spa Simulator",
	} {
		if got := Parse(raw).Base; got != want {
			t.Errorf("Parse(%q).Base = %q, want %q", raw, got, want)
		}
	}
	// Настоящий языковой маркер по-прежнему снимается.
	for _, raw := range []string{"Silent Hill 2 (2024) [RUS]", "Silent Hill 2 [ENG/RUS] v1.0", "Silent Hill 2 - Rus"} {
		p := Parse(raw)
		if p.Base != "Silent Hill 2" || len(p.Languages) == 0 {
			t.Errorf("Parse(%q) = base %q langs %v", raw, p.Base, p.Languages)
		}
	}
}
