package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
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
