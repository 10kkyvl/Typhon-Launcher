package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestPresetsForkAndRoundTrip(t *testing.T) {
	for _, preset := range presets {
		t.Run(preset.ID, func(t *testing.T) {
			dir := t.TempDir()
			s := mustServiceAt(t, filepath.Join(dir, "themes.json"))

			fork := preset
			fork.BuiltIn = false
			fork.Tokens = map[string]string{}
			for name, value := range preset.Tokens {
				if isSettingsOwned(name) {
					continue
				}
				fork.Tokens[name] = value
			}
			fork.Tokens["--accent"] = "#123456"
			saved, err := s.Save(fork)
			if err != nil {
				t.Fatalf("Save(fork of %s) error = %v", preset.ID, err)
			}
			if saved.ID == preset.ID || saved.BuiltIn {
				t.Fatalf("Save(fork of %s) = id %q builtIn %v, want a new user theme", preset.ID, saved.ID, saved.BuiltIn)
			}

			exportPath := filepath.Join(dir, preset.ID+".typhontheme")
			if err := s.Export(preset.ID, exportPath); err != nil {
				t.Fatalf("Export(%s) error = %v", preset.ID, err)
			}
			raw, err := os.ReadFile(filepath.Clean(exportPath))
			if err != nil {
				t.Fatal(err)
			}
			var payload file
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("exported file is not a theme file: %v", err)
			}
			for _, name := range settingsOwnedTokens {
				if _, ok := payload.Theme.Tokens[name]; ok {
					t.Errorf("exported %s carries settings-owned token %s", preset.ID, name)
				}
			}

			imported, err := s.Import(exportPath)
			if err != nil {
				t.Fatalf("Import(export of %s) error = %v", preset.ID, err)
			}
			if imported.BuiltIn || imported.Base != preset.Base || imported.Name != preset.Name {
				t.Fatalf("Import(export of %s) = %+v", preset.ID, imported)
			}
			if len(imported.Tokens) != len(fork.Tokens) {
				t.Fatalf("Import(export of %s) has %d tokens, want %d", preset.ID, len(imported.Tokens), len(fork.Tokens))
			}
			for name, value := range preset.Tokens {
				if isSettingsOwned(name) {
					continue
				}
				if imported.Tokens[name] != value {
					t.Errorf("Import(export of %s) token %s = %q, want %q", preset.ID, name, imported.Tokens[name], value)
				}
			}
			got, err := s.Get(preset.ID)
			if err != nil || !got.BuiltIn || got.Tokens["--ui-scale"] == "" {
				t.Fatalf("preset %s changed by fork, export and import: %+v, err=%v", preset.ID, got, err)
			}
		})
	}
}

func TestBuiltInPresetsListedInOrder(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	want := []string{"dark", "light", "contrast", "blush"}
	list := s.List()
	if len(list) != len(want) {
		t.Fatalf("List() = %d themes, want the %d built-ins %v", len(list), len(want), want)
	}
	for i, id := range want {
		if list[i].ID != id || !list[i].BuiltIn {
			t.Errorf("List()[%d] = %q builtIn %v, want built-in %q", i, list[i].ID, list[i].BuiltIn, id)
		}
	}
	blush, err := s.Get("blush")
	if err != nil {
		t.Fatalf("Get(blush) error = %v", err)
	}
	if blush.Base != BaseLight {
		t.Errorf("blush base = %q, want %q", blush.Base, BaseLight)
	}
}

var shadowColor = regexp.MustCompile(`rgba?\([^)]*\)|#[0-9a-fA-F]+`)

// How far the backdrop is dimmed toward the page is tuned with the palette:
// a light page needs more of itself over the art than a dark one.
var paletteNumbers = map[string]bool{"--ambient-dim": true}

// Presets differ in colour only: shape, motion and type are one design, so a
// value tuned in one preset and forgotten in another shows up here.
func TestPresetsShareEverythingButColour(t *testing.T) {
	for _, preset := range presets {
		t.Run(preset.ID, func(t *testing.T) {
			for _, tok := range allowedTokens {
				value, ok := preset.Tokens[tok.Name]
				if !ok {
					t.Errorf("%s is missing", tok.Name)
					continue
				}
				if !validTokenValue(tok.Name, tok.Kind, value) {
					t.Errorf("%s = %q is not a valid %s", tok.Name, value, tok.Kind)
				}
				dark := presetDark.Tokens[tok.Name]
				switch {
				case tok.Kind == KindColor || paletteNumbers[tok.Name]:
				case tok.Kind == KindShadow:
					if got, want := shadowColor.ReplaceAllString(value, "C"), shadowColor.ReplaceAllString(dark, "C"); got != want {
						t.Errorf("%s geometry = %q, dark has %q", tok.Name, got, want)
					}
				default:
					if value != dark {
						t.Errorf("%s = %q, dark has %q", tok.Name, value, dark)
					}
				}
			}
			for name := range preset.Tokens {
				if _, ok := allowedTokenSet[name]; !ok && !isSettingsOwned(name) {
					t.Errorf("%s is not a theme token", name)
				}
			}
		})
	}
}
