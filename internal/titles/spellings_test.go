package titles

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestMatchNamesAppendsEnglishSpelling(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"Дед Спейс 2", []string{"Dead Space 2"}},
		{"Дед Спейс 1", []string{"Dead Space"}},
		{"Ассасин Крид Ориджин", []string{"Assassin's Creed Origins"}},
		{"Ассасин Крид 2", []string{"Assassin's Creed II"}},
		{"Ассасин Крид Director's Cut Edition", []string{"Assassin's Creed Director's Cut Edition"}},
		{"Нид фор Спид Карбон", []string{"Need for Speed: Carbon"}},
		{"Нид Фор Спид Прострит", []string{"Need for Speed: ProStreet"}},
		{"Бэтмен Аркхем Сити", []string{"Batman: Arkham City"}},
		{"Цивилизация 5", []string{"Sid Meier's Civilization V"}},
		{"Цивилизация VI", []string{"Sid Meier's Civilization VI"}},
		{"Якудза 0 (2018)", []string{"Yakuza 0"}},
		{"Дизонорд Death of the Outsider", []string{"Dishonored Death of the Outsider"}},
		{"Резидент Ивел 3 Ремейк (2020)", []string{"Resident Evil 3 Remake", "Resident Evil 3"}},
		{"Дум 2016", []string{"Doom 2016", "Doom"}},
		{"ПЕС 2013", []string{"Pro Evolution Soccer 2013", "Pro Evolution Soccer"}},
		{"Кал оф Дьюти Модерн Варфаер 3 + Multiplayer", []string{"Call of Duty: Modern Warfare 3 Multiplayer", "Call of Duty: Modern Warfare 3"}},
		{"Ведьмак 3 Дикая охота", []string{"The Witcher 3: Wild Hunt"}},
		{"Вольфенштейн 2009", []string{"Wolfenstein"}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := MatchNames(tc.raw)
			base := matchNamesMixed(tc.raw)
			if len(got) < len(base) || !slices.Equal(got[:len(base)], base) {
				t.Fatalf("MatchNames(%q) = %q, old variants %q must stay first", tc.raw, got, base)
			}
			extra := got[len(base):]
			if !slices.Equal(extra, tc.want) {
				t.Fatalf("MatchNames(%q) extra = %q, want %q", tc.raw, extra, tc.want)
			}
		})
	}
}

func TestMatchNamesSpellingMissesChangeNothing(t *testing.T) {
	for _, raw := range []string{
		"Иван Царевич и Серый волк",
		"Нид Фор Спид Новое Название",
		"Ассасин Крид Неизвестная Часть",
		"Dead Space 2",
		"Resident Evil 3 (2020)",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			got, base := MatchNames(raw), matchNamesMixed(raw)
			if !slices.Equal(got, base) {
				t.Fatalf("MatchNames(%q) = %q, want unchanged %q", raw, got, base)
			}
		})
	}
}

func TestSpellingTailRejectsReleaseNoise(t *testing.T) {
	d := Active()
	for _, name := range []string{
		"Драгон Эйдж Ориджин gog 0 8",
		"Дед Спейс 2 l",
		"Дед Спейс 2 RUS",
		"Дед Спейс 2 x64",
		"Дед Спейс 2 Неизвестное Слово",
	} {
		if got := d.spellingNames([]string{name}); len(got) != 0 {
			t.Errorf("spellingNames(%q) = %q, want none", name, got)
		}
	}
}

func TestSpellingNamesLongestPrefixWins(t *testing.T) {
	spec := Spec{Version: DictVersion, TitleSpellings: map[string]string{
		"нид фор спид":             "Need for Speed",
		"нид фор спид хот":         "Wrong Short",
		"нид фор спид хот пурсуит": "Need for Speed: Hot Pursuit",
	}}
	d, err := NewDict(spec)
	if err != nil {
		t.Fatal(err)
	}
	got := d.spellingNames([]string{"Нид Фор Спид Хот Пурсуит 2"})
	if want := []string{"Need for Speed: Hot Pursuit 2"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := d.spellingNames([]string{"Нид Фор Спид Прострит"}); len(got) != 0 {
		t.Fatalf("an unknown word after a known prefix gave %q, want none", got)
	}
}

func TestSpellingsDoNotDisplaceOldVariants(t *testing.T) {
	old := []string{"Дед Спейс 2 A", "Дед Спейс 2 B", "Дед Спейс 2 C", "Дед Спейс 2 D", "Дед Спейс 2 E", "Дед Спейс 2 F"}
	got := withSpellings(old)
	if !slices.Equal(got, old) {
		t.Fatalf("withSpellings = %q, want the six old variants untouched", got)
	}
	if len(withSpellings([]string{"Дед Спейс 2", "x"})) > maxMatchNames {
		t.Fatalf("result exceeds %d variants", maxMatchNames)
	}
}

func TestSpellingsAcceptLayerWithoutField(t *testing.T) {
	spec, err := ParseSpec([]byte(`{"version":1,"langCodes":["RUS"]}`))
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	merged := builtin.Merge(spec)
	if len(merged.TitleSpellings) != len(builtin.TitleSpellings) || len(merged.TitleSpellings) == 0 {
		t.Fatalf("a layer without the field changed spellings: %d -> %d", len(builtin.TitleSpellings), len(merged.TitleSpellings))
	}
}

func TestSpellingsMergeOverridesAndDeletes(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	layer, err := ParseSpec([]byte(`{"version":1,"titleSpellings":{"дед спейс":"Dead Space (2008)","дезлуп":"","Нова Игра":"New Game"}}`))
	if err != nil {
		t.Fatal(err)
	}
	merged := builtin.Merge(layer)
	if merged.TitleSpellings["дед спейс"] != "Dead Space (2008)" {
		t.Errorf("override lost: %q", merged.TitleSpellings["дед спейс"])
	}
	if _, ok := merged.TitleSpellings["дезлуп"]; ok {
		t.Error("an empty value must delete the key")
	}
	if merged.TitleSpellings["Нова Игра"] != "New Game" {
		t.Error("new key missing")
	}
	if builtin.TitleSpellings["дезлуп"] == "" {
		t.Error("merge mutated the builtin spec")
	}
}

func TestParseSpecValidatesSpellings(t *testing.T) {
	many := map[string]string{}
	for i := range MaxSpellings + 1 {
		many["игра "+strconv.Itoa(i)] = "Game " + strconv.Itoa(i)
	}
	manyJSON, err := json.Marshal(map[string]any{"version": 1, "titleSpellings": many})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data string
		want error
	}{
		{"broken json", `{"version":1,"titleSpellings":{"а":`, nil},
		{"wrong type", `{"version":1,"titleSpellings":["a"]}`, nil},
		{"blank key", `{"version":1,"titleSpellings":{"  ":"Game"}}`, ErrDictEntry},
		{"key without words", `{"version":1,"titleSpellings":{"!!!":"Game"}}`, ErrDictEntry},
		{"too many key words", `{"version":1,"titleSpellings":{"а б в г д е ж з и к л м н":"Game"}}`, ErrDictEntry},
		{"overlong value", `{"version":1,"titleSpellings":{"игра":"` + strings.Repeat("a", MaxDictToken+1) + `"}}`, ErrDictEntry},
		{"cyrillic value", `{"version":1,"titleSpellings":{"игра":"Игра"}}`, ErrDictEntry},
		{"link value", `{"version":1,"titleSpellings":{"игра":"https://example.com/game"}}`, ErrDictEntry},
		{"control character", `{"version":1,"titleSpellings":{"игра":"Game\u0007"}}`, ErrDictEntry},
		{"blank variant", `{"version":1,"titleSpellings":{"игра":"Game | "}}`, ErrDictEntry},
		{"too many entries", string(manyJSON), ErrDictTooLarge},
		{"oversized body", `{"version":1,"titleSpellings":{"игра":"` + strings.Repeat("a", MaxDictBytes) + `"}}`, ErrDictTooLarge},
		{"empty field is fine", `{"version":1,"titleSpellings":{}}`, errNone},
		{"empty value is fine", `{"version":1,"titleSpellings":{"игра":""}}`, errNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSpec([]byte(tc.data))
			if errors.Is(tc.want, errNone) {
				if err != nil {
					t.Fatalf("ParseSpec error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ParseSpec = nil error, want a failure")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("ParseSpec error = %v, want %v", err, tc.want)
			}
		})
	}
}

var errNone = errors.New("no error expected")

func TestBuiltinSpellingsAreValid(t *testing.T) {
	spec, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.TitleSpellings) == 0 {
		t.Fatal("builtin has no spellings")
	}
	seen := map[string]string{}
	for key := range spec.TitleSpellings {
		norm := spellingKey(key)
		if other, dup := seen[norm]; dup {
			t.Errorf("keys %q and %q normalize to the same phrase", key, other)
		}
		seen[norm] = key
	}
}
