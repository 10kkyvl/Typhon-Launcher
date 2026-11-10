package theme

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

// editThemesFile rewrites every stored theme entry of themes.json in place,
// keeping the envelope version, pending and confirmed as they are.
func editThemesFile(t *testing.T, path string, edit func(entry map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int            `json:"version"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("themes.json is not a state envelope: %v", err)
	}
	themes, ok := doc.Data["themes"].([]any)
	if !ok {
		t.Fatalf("themes.json has no theme list: %s", raw)
	}
	for _, item := range themes {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("theme entry is not an object: %v", item)
		}
		edit(entry)
	}
	if err := storage.Save(path, doc.Version, doc.Data); err != nil {
		t.Fatal(err)
	}
}

// dropContracts is what a 1.x launcher does to every theme when it writes
// themes.json after a downgrade.
func dropContracts(t *testing.T, path string) {
	t.Helper()
	editThemesFile(t, path, func(entry map[string]any) { delete(entry, "contract") })
}

func requireStoredContract(t *testing.T, path, id string, want int) {
	t.Helper()
	_, stored := readThemesFile(t, path)
	entry, ok := stored[id]
	if !ok {
		t.Fatalf("themes.json has no %s", id)
	}
	if entry.Contract != want {
		t.Errorf("themes.json %s contract = %d, want %d", id, entry.Contract, want)
	}
}

func TestDowngradeRoundTripDoesNotScaleAgain(t *testing.T) {
	cases := []struct {
		name string
		fork Theme
	}{
		{"dark", oldContractFork("fork", "Моя тема", BaseDark,
			withChanges(fixture141(t, "dark"), map[string]string{"--accent": "#ff0055", "--radius-md": "1rem"}))},
		{"light", oldContractFork("fork", "Моя тема", BaseLight,
			withChanges(fixture141(t, "light"), map[string]string{"--accent": "#ff0055", "--space-3": "2rem"}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeStoredThemes(t, []Theme{tc.fork}, "")
			s := mustServiceAt(t, path)
			migrated, err := s.Get("fork")
			if err != nil {
				t.Fatalf("Get(fork) error = %v", err)
			}
			want := maps.Clone(migrated.Tokens)
			for round := 1; round <= 3; round++ {
				if _, err := s.Save(validTheme("other")); err != nil {
					t.Fatalf("round %d: Save() error = %v", round, err)
				}
				requireStoredContract(t, path, "fork", 2)
				dropContracts(t, path)

				s = mustServiceAt(t, path)
				got, err := s.Get("fork")
				if err != nil {
					t.Fatalf("round %d: Get(fork) error = %v", round, err)
				}
				requireTokens(t, "after a 1.x write", got.Tokens, want, "")
				requireContract2(t, "after a 1.x write", got)
			}
		})
	}
}

func TestOneUserColourOnTheCurrentPresetIsOnlyStamped(t *testing.T) {
	tokens := currentWith(t, "dark", map[string]string{"--accent": "#ff0055"})
	got := migratedFrom(t, BaseDark, tokens)
	requireTokens(t, "marker-less current theme", got.Tokens, tokens, "")
	requireContract2(t, "marker-less current theme", got)
}

func TestNewerContractFileIsRefused(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, s *Service, text string) (Theme, error)
	}{
		{"ImportText", func(t *testing.T, s *Service, text string) (Theme, error) { return s.ImportText(text, "") }},
		{"Import", func(t *testing.T, s *Service, text string) (Theme, error) {
			return s.Import(importFilePath(t, "future.typhontheme", []byte(text)))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th := validTheme("future")
			th.Contract = currentContract + 1
			raw, err := json.Marshal(file{Version: 1, Theme: th})
			if err != nil {
				t.Fatal(err)
			}
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			if _, err := tc.run(t, s, string(raw)); !errors.Is(err, ErrThemeVersion) {
				t.Fatalf("import of a contract %d theme error = %v, want %v", th.Contract, err, ErrThemeVersion)
			}
			if _, err := s.Get("future"); !errors.Is(err, ErrThemeNotFound) {
				t.Fatalf("refused theme is stored: Get error = %v", err)
			}
		})
	}
}

func TestStoredNewerContractThemeIsKept(t *testing.T) {
	future := validTheme("future")
	future.Tokens = map[string]string{"--accent": "#ff0055", "--radius-md": "1rem"}
	future.Contract = currentContract + 1
	path := writeStoredThemes(t, []Theme{future}, "")

	s := mustServiceAt(t, path)
	got, err := s.Get("future")
	if err != nil {
		t.Fatalf("Get(future) error = %v", err)
	}
	requireTokens(t, "served", got.Tokens, future.Tokens, "")
	if got.Contract != future.Contract {
		t.Errorf("served contract = %d, want %d", got.Contract, future.Contract)
	}

	if _, err := s.Save(validTheme("other")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	requireStoredContract(t, path, "future", future.Contract)
	_, stored := readThemesFile(t, path)
	requireTokens(t, "persisted", stored["future"].Tokens, future.Tokens, "")

	again, err := mustServiceAt(t, path).Get("future")
	if err != nil {
		t.Fatalf("Get(future) after restart error = %v", err)
	}
	requireTokens(t, "after restart", again.Tokens, future.Tokens, "")
	if again.Contract != future.Contract {
		t.Errorf("contract after restart = %d, want %d", again.Contract, future.Contract)
	}
}

// The pending-theme revert is the one write at startup; it stores the
// themes as served, so an old fork reaches disk migrated.
func TestPendingRevertAtStartupWritesMigratedThemes(t *testing.T) {
	fork, kept := darkFork1x(t, "fork", "Моя тема")
	path := filepath.Join(t.TempDir(), "themes.json")
	if err := storage.Save(path, stateVersion, state{Themes: []Theme{fork}, Pending: "fork"}); err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, path)
	active, err := s.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active.ID != defaultTheme().ID {
		t.Errorf("active after startup = %q, want the default %q", active.ID, defaultTheme().ID)
	}

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Data struct {
			Pending   string `json:"pending"`
			Confirmed string `json:"confirmed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("themes.json is not a state envelope: %v", err)
	}
	if doc.Data.Pending != "" || doc.Data.Confirmed != "" {
		t.Errorf("themes.json pending %q confirmed %q, want both empty", doc.Data.Pending, doc.Data.Confirmed)
	}
	requireStoredContract(t, path, "fork", 2)
	_, stored := readThemesFile(t, path)
	requireTokens(t, "written at startup", stored["fork"].Tokens, wantAfterMigration(t, "dark", fork.Tokens, kept), "")
}
