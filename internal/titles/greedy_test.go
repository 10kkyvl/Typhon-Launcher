package titles

import "testing"

// Разбор режет из названия версии, издания, языки и скобки. Каждое такое
// правило — шанс срезать слишком много и схлопнуть две разные игры в одну.
func TestSequelsAndEpisodesStayApart(t *testing.T) {
	groups := [][]string{
		{"Half-Life 2", "Half-Life 2: Episode One", "Half-Life 2: Episode Two"},
		{"Portal", "Portal 2"},
		{"The Witcher 3: Wild Hunt", "The Witcher 3: Wild Hunt - Blood and Wine"},
		{"Dark Souls II", "Dark Souls III"},
		{"Left 4 Dead", "Left 4 Dead 2"},
		{"Deus Ex: Human Revolution", "Deus Ex: Mankind Divided"},
	}

	for _, group := range groups {
		bases := make(map[string]string, len(group))
		norms := make(map[string]string, len(group))
		for _, raw := range group {
			p := Parse(raw)
			if p.Base == "" {
				t.Fatalf("Parse(%q).Base is empty", raw)
			}
			if other, ok := bases[p.Base]; ok {
				t.Errorf("Parse(%q).Base == Parse(%q).Base == %q", raw, other, p.Base)
			}
			if other, ok := norms[p.Normalized]; ok {
				t.Errorf("Parse(%q).Normalized == Parse(%q).Normalized == %q", raw, other, p.Normalized)
			}
			bases[p.Base] = raw
			norms[p.Normalized] = raw
		}
	}
}

// Издание и версия срезаются, номер части и подзаголовок — нет.
func TestTrailingScanStopsAtTheTitle(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"edition after a numbered sequel", "Half-Life 2 Deluxe Edition", "Half Life 2"},
		{"repack tag after an episode", "Half-Life 2: Episode One [FitGirl Repack]", "Half Life 2: Episode One"},
		{"version after a subtitle", "Half-Life 2: Episode Two v1.0.4", "Half Life 2: Episode Two"},
		{"a title that is only an edition word survives", "Remastered", "Remastered"},
		{"a title ending in a tag word survives", "Portal", "Portal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.raw).Base; got != tc.want {
				t.Fatalf("Parse(%q).Base = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// Год нужен для тёзок, поэтому он вырезается из названия, но не выбрасывается.
func TestYearIsCutOutAndKept(t *testing.T) {
	cases := []struct {
		raw       string
		wantBase  string
		wantYear  int
		wantEqual string
	}{
		{"Resident Evil 4 (2005)", "Resident Evil 4", 2005, "resident evil 4"},
		{"Resident Evil 4 (2023) [FitGirl Repack]", "Resident Evil 4", 2023, "resident evil 4"},
		{"Prey (2017) [MULTi9] v1.0", "Prey", 2017, "prey"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := Parse(tc.raw)
			if p.Base != tc.wantBase {
				t.Errorf("Base = %q, want %q", p.Base, tc.wantBase)
			}
			if p.Year != tc.wantYear {
				t.Errorf("Year = %d, want %d", p.Year, tc.wantYear)
			}
			if p.Normalized != tc.wantEqual {
				t.Errorf("Normalized = %q, want %q", p.Normalized, tc.wantEqual)
			}
		})
	}
}

// «(+ all DLC)» раньше не опознавалось как известная группа и целиком
// оставалось в названии: репак базовой игры уходил в ручную проверку вместо
// точного совпадения.
func TestBracketFillerDoesNotLeakIntoTheTitle(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"Half-Life 2 [FitGirl Repack] (v1.0 + all DLC)", "Half Life 2"},
		{"Half-Life 2 (+ all DLC)", "Half Life 2"},
		{"Elden Ring (incl. DLC)", "Elden Ring"},
		{"Portal 2 (with all DLC)", "Portal 2"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			p := Parse(tc.raw)
			if p.Base != tc.want {
				t.Fatalf("Base = %q, want %q", p.Base, tc.want)
			}
			if p.Normalized != Normalize(tc.want) {
				t.Fatalf("Normalized = %q, want %q", p.Normalized, Normalize(tc.want))
			}
		})
	}
}

// Слово из списка филлеров не должно съедать настоящее название.
func TestBracketFillerDoesNotEatRealTitles(t *testing.T) {
	for _, raw := range []string{"All Stars", "Kingdom Come: Deliverance (All Stars)"} {
		if got := Parse(raw).Base; got == "" {
			t.Fatalf("Parse(%q).Base is empty", raw)
		}
	}
	if got := Parse("Kingdom Come: Deliverance (All Stars)").Base; got != "Kingdom Come: Deliverance (All Stars)" {
		t.Fatalf("Base = %q, want the bracket kept", got)
	}
}
