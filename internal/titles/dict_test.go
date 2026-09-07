package titles

import (
	"errors"
	"strings"
	"testing"
)

func TestBuiltinDictIsValid(t *testing.T) {
	if err := Ready(); err != nil {
		t.Fatalf("Ready() = %v, want nil", err)
	}
	spec, err := Builtin()
	if err != nil {
		t.Fatalf("Builtin() error = %v", err)
	}
	if spec.Version != DictVersion {
		t.Errorf("Version = %d, want %d", spec.Version, DictVersion)
	}
	if len(spec.RepackerPriority) == 0 {
		t.Error("RepackerPriority is empty")
	}
	if len(spec.GameTypes) == 0 {
		t.Error("GameTypes is empty")
	}
	if Active() == nil {
		t.Fatal("Active() = nil")
	}
}

func TestParseSpecRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		data string
		want error
	}{
		{"broken json", `{"version":1,`, nil},
		{"not an object", `"a string"`, nil},
		{"zero version", `{"version":0}`, ErrDictVersion},
		{"future version", `{"version":99}`, ErrDictVersion},
		{"blank list entry", `{"version":1,"langCodes":["RUS","  "]}`, ErrDictEntry},
		{"blank map key", `{"version":1,"releaseTags":{"  ":"repack"}}`, ErrDictEntry},
		{"overlong token", `{"version":1,"langCodes":["` + strings.Repeat("a", MaxDictToken+1) + `"]}`, ErrDictEntry},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSpec([]byte(tc.data))
			if err == nil {
				t.Fatalf("ParseSpec(%q) = nil error, want a failure", tc.data)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("ParseSpec error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestParseSpecRejectsOversizedBody(t *testing.T) {
	body := make([]byte, MaxDictBytes+1)
	for i := range body {
		body[i] = ' '
	}
	if _, err := ParseSpec(body); !errors.Is(err, ErrDictTooLarge) {
		t.Fatalf("ParseSpec error = %v, want ErrDictTooLarge", err)
	}
}

func TestMergeReplacesListsAndMergesTables(t *testing.T) {
	base := Spec{
		Version:          DictVersion,
		LangCodes:        []string{"RUS", "ENG"},
		EditionPhrases:   []string{"Deluxe Edition"},
		RepackerPriority: []string{"fitgirl", "dodi"},
		ReleaseTags:      map[string]string{"fitgirl": "fitgirl", "codex": "codex"},
	}

	t.Run("absent fields keep the base", func(t *testing.T) {
		got := base.Merge(Spec{Version: DictVersion})
		if len(got.LangCodes) != 2 || len(got.RepackerPriority) != 2 {
			t.Fatalf("merged = %+v", got)
		}
		if got.ReleaseTags["codex"] != "codex" {
			t.Fatalf("ReleaseTags = %+v", got.ReleaseTags)
		}
	})

	t.Run("lists are replaced whole", func(t *testing.T) {
		got := base.Merge(Spec{Version: DictVersion, RepackerPriority: []string{"dodi"}})
		if len(got.RepackerPriority) != 1 || got.RepackerPriority[0] != "dodi" {
			t.Fatalf("RepackerPriority = %v, want [dodi]", got.RepackerPriority)
		}
	})

	t.Run("tables merge by key", func(t *testing.T) {
		got := base.Merge(Spec{Version: DictVersion, ReleaseTags: map[string]string{"newgroup": "newgroup"}})
		if got.ReleaseTags["fitgirl"] != "fitgirl" || got.ReleaseTags["newgroup"] != "newgroup" {
			t.Fatalf("ReleaseTags = %+v", got.ReleaseTags)
		}
	})

	t.Run("an empty value removes a key", func(t *testing.T) {
		got := base.Merge(Spec{Version: DictVersion, ReleaseTags: map[string]string{"codex": ""}})
		if _, ok := got.ReleaseTags["codex"]; ok {
			t.Fatalf("ReleaseTags still has codex: %+v", got.ReleaseTags)
		}
		if got.ReleaseTags["fitgirl"] != "fitgirl" {
			t.Fatalf("removing one key dropped another: %+v", got.ReleaseTags)
		}
	})

	t.Run("the base is left alone", func(t *testing.T) {
		base.Merge(Spec{Version: DictVersion, ReleaseTags: map[string]string{"codex": ""}})
		if base.ReleaseTags["codex"] != "codex" {
			t.Fatalf("Merge mutated the receiver: %+v", base.ReleaseTags)
		}
	})
}

// Новый репакер должен появляться словарём, без релиза лаунчера: это вся
// причина, по которой списки уехали из кода.
func TestDictLearnsANewRepackerFromALayer(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatalf("Builtin() error = %v", err)
	}
	merged := builtin.Merge(Spec{
		Version:          DictVersion,
		ReleaseTags:      map[string]string{"someguy": "someguy"},
		RepackerPriority: []string{"someguy", "fitgirl"},
	})
	dict, err := NewDict(merged)
	if err != nil {
		t.Fatalf("NewDict() error = %v", err)
	}

	parsed := dict.Parse("Portal 2 [SomeGuy Repack]")
	if parsed.Base != "Portal 2" {
		t.Errorf("Base = %q, want Portal 2", parsed.Base)
	}
	if got := dict.Repacker(parsed.Tags); got != "someguy" {
		t.Errorf("Repacker = %q, want someguy; tags = %v", got, parsed.Tags)
	}

	if Active().Repacker([]string{"someguy"}) != "" {
		t.Error("the built-in dictionary already knows someguy, the case proves nothing")
	}
}

func TestEmptyDictDoesNotEatTitles(t *testing.T) {
	dict, err := NewDict(Spec{Version: DictVersion})
	if err != nil {
		t.Fatalf("NewDict() error = %v", err)
	}
	parsed := dict.Parse("Half-Life 2")
	if parsed.Base != "Half Life 2" {
		t.Fatalf("Base = %q, want Half Life 2", parsed.Base)
	}
	if len(parsed.Languages) != 0 {
		t.Fatalf("Languages = %v, want none from an empty dictionary", parsed.Languages)
	}
}

func TestIsGameType(t *testing.T) {
	dict, err := NewDict(Spec{Version: DictVersion, GameTypes: []string{"Main Game", "Remake"}})
	if err != nil {
		t.Fatalf("NewDict() error = %v", err)
	}
	cases := []struct {
		kind string
		want bool
	}{
		{"Main Game", true},
		{"main game", true},
		{"  Main   Game ", true},
		{"Remake", true},
		{"DLC", false},
		{"Bundle", false},
		{"", true},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			if got := dict.IsGameType(tc.kind); got != tc.want {
				t.Fatalf("IsGameType(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}

	empty, err := NewDict(Spec{Version: DictVersion})
	if err != nil {
		t.Fatalf("NewDict() error = %v", err)
	}
	if !empty.IsGameType("DLC") {
		t.Error("a dictionary without a type list must not filter anything out")
	}
}

func TestBuiltinGameTypesKeepGamesAndDropAddons(t *testing.T) {
	games := []string{"Main Game", "Remake", "Remaster", "Port", "Standalone Expansion", "Expanded Game", "Fork"}
	addons := []string{"DLC", "Expansion", "Bundle", "Mod", "Episode", "Season", "Pack", "Update"}
	for _, kind := range games {
		if !IsGameType(kind) {
			t.Errorf("IsGameType(%q) = false, want true", kind)
		}
	}
	for _, kind := range addons {
		if IsGameType(kind) {
			t.Errorf("IsGameType(%q) = true, want false", kind)
		}
	}
}
