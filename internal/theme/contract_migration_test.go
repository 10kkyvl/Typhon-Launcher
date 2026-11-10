package theme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"typhon/internal/storage"
)

var fixtureForkTime = time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

// fixtureLengths holds the length tokens shared unchanged by every preset
// from launcher 0.4 to 1.4.1, when the root font was 11px.
func fixtureLengths() map[string]string {
	return map[string]string{
		"--radius-xs":        "0.4rem",
		"--radius-sm":        "0.6rem",
		"--radius-md":        "0.8rem",
		"--radius-lg":        "1.2rem",
		"--radius-xl":        "1.6rem",
		"--cut":              "0.2rem",
		"--space-1":          "0.4rem",
		"--space-2":          "0.8rem",
		"--space-3":          "1.2rem",
		"--space-4":          "1.6rem",
		"--space-5":          "2rem",
		"--space-6":          "2.4rem",
		"--space-8":          "3.2rem",
		"--space-10":         "4rem",
		"--space-12":         "4.8rem",
		"--page-x":           "3.6rem",
		"--page-max":         "232rem",
		"--prose-max":        "76rem",
		"--sidebar-w":        "23.2rem",
		"--topbar-h":         "5.6rem",
		"--font-xs":          "1.3rem",
		"--font-sm":          "1.4rem",
		"--font-md":          "1.5rem",
		"--font-lg":          "1.7rem",
		"--font-xl":          "1.9rem",
		"--font-title":       "3.2rem",
		"--font-hero":        "clamp(3.6rem, 2.6vw, 4.4rem)",
		"--tracking-title":   "-0.024em",
		"--tracking-heading": "-0.012em",
		"--control-sm":       "3.2rem",
		"--control-md":       "3.8rem",
		"--control-lg":       "4.4rem",
	}
}

// fixture141 is a fork of a built-in preset as launcher 1.0.0–1.4.1 stored
// it: every preset token except the settings-owned --ui-scale.
func fixture141(t *testing.T, id string) map[string]string {
	t.Helper()
	tokens := fixtureLengths()
	maps.Copy(tokens, map[string]string{
		"--font-sans":   "Inter Tight Variable, Inter, Segoe UI, system-ui, sans-serif",
		"--ease":        "cubic-bezier(0.2, 0.8, 0.2, 1)",
		"--ease-spring": "cubic-bezier(0.34, 1.4, 0.64, 1)",
		"--dur":         "160ms",
		"--dur-fast":    "120ms",
		"--dur-panel":   "180ms",
		"--dur-slow":    "320ms",
	})
	switch id {
	case "dark":
		maps.Copy(tokens, map[string]string{
			"--bg":             "#090d17",
			"--bg-sidebar":     "#090d17",
			"--surface":        "#0f1522",
			"--surface-2":      "#131a29",
			"--surface-3":      "#1a2233",
			"--surface-4":      "#222b3f",
			"--border":         "rgba(255, 255, 255, 0.07)",
			"--border-strong":  "rgba(255, 255, 255, 0.12)",
			"--hover":          "rgba(255, 255, 255, 0.045)",
			"--hover-strong":   "rgba(255, 255, 255, 0.075)",
			"--text":           "#eef2f6",
			"--text-2":         "#9aa5b8",
			"--text-3":         "#647089",
			"--accent":         "#4f6bff",
			"--accent-hover":   "#6480ff",
			"--accent-subtle":  "rgba(79, 107, 255, 0.16)",
			"--accent-text":    "#93a4ff",
			"--accent-ring":    "rgba(79, 107, 255, 0.5)",
			"--success":        "#65b883",
			"--success-subtle": "rgba(101, 184, 131, 0.13)",
			"--warning":        "#d8a45d",
			"--warning-subtle": "rgba(216, 164, 93, 0.13)",
			"--danger":         "#d96969",
			"--danger-subtle":  "rgba(217, 105, 105, 0.12)",
			"--shadow-pop":     "0 0.8rem 2.8rem rgba(0, 0, 0, 0.45), 0 2px 0.8rem rgba(0, 0, 0, 0.3)",
			"--shadow-modal":   "0 2rem 6rem rgba(0, 0, 0, 0.55)",
			"--shadow-lift":    "0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.6)",
		})
	case "light":
		maps.Copy(tokens, map[string]string{
			"--bg":             "#eef1f4",
			"--bg-sidebar":     "#eef1f4",
			"--surface":        "#f8f9fb",
			"--surface-2":      "#ffffff",
			"--surface-3":      "#e9ecf1",
			"--surface-4":      "#dfe3ea",
			"--border":         "rgba(0, 0, 0, 0.1)",
			"--border-strong":  "rgba(0, 0, 0, 0.16)",
			"--hover":          "rgba(0, 0, 0, 0.04)",
			"--hover-strong":   "rgba(0, 0, 0, 0.07)",
			"--text":           "#10161d",
			"--text-2":         "#4b5563",
			"--text-3":         "#7a8494",
			"--accent":         "#2d3fed",
			"--accent-hover":   "#1327e4",
			"--accent-subtle":  "rgba(45, 63, 237, 0.1)",
			"--accent-text":    "#1225d8",
			"--accent-ring":    "rgba(45, 63, 237, 0.45)",
			"--success":        "#2f8f5b",
			"--success-subtle": "rgba(47, 143, 91, 0.12)",
			"--warning":        "#b3791f",
			"--warning-subtle": "rgba(179, 121, 31, 0.12)",
			"--danger":         "#c53c3c",
			"--danger-subtle":  "rgba(197, 60, 60, 0.1)",
			"--shadow-pop":     "0 0.8rem 2.8rem rgba(15, 23, 32, 0.12), 0 2px 0.8rem rgba(15, 23, 32, 0.08)",
			"--shadow-modal":   "0 2rem 6rem rgba(15, 23, 32, 0.16)",
			"--shadow-lift":    "0 1.2rem 2.8rem -0.8rem rgba(15, 23, 32, 0.22)",
		})
	case "contrast":
		maps.Copy(tokens, map[string]string{
			"--bg":             "#000000",
			"--bg-sidebar":     "#000000",
			"--surface":        "#0d0d0d",
			"--surface-2":      "#141414",
			"--surface-3":      "#1c1c1c",
			"--surface-4":      "#262626",
			"--border":         "rgba(255, 255, 255, 0.28)",
			"--border-strong":  "rgba(255, 255, 255, 0.45)",
			"--hover":          "rgba(255, 255, 255, 0.12)",
			"--hover-strong":   "rgba(255, 255, 255, 0.18)",
			"--text":           "#ffffff",
			"--text-2":         "#d8dee5",
			"--text-3":         "#aab2bd",
			"--accent":         "#4f6bff",
			"--accent-hover":   "#6480ff",
			"--accent-subtle":  "rgba(79, 107, 255, 0.16)",
			"--accent-text":    "#93a4ff",
			"--accent-ring":    "rgba(79, 107, 255, 0.5)",
			"--success":        "#7fe0a6",
			"--success-subtle": "rgba(127, 224, 166, 0.2)",
			"--warning":        "#f0bd6c",
			"--warning-subtle": "rgba(240, 189, 108, 0.2)",
			"--danger":         "#ff8a8a",
			"--danger-subtle":  "rgba(255, 138, 138, 0.2)",
			"--shadow-pop":     "0 0.8rem 2.8rem rgba(0, 0, 0, 0.7), 0 2px 0.8rem rgba(0, 0, 0, 0.5)",
			"--shadow-modal":   "0 2rem 6rem rgba(0, 0, 0, 0.8)",
			"--shadow-lift":    "0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.85)",
		})
	default:
		t.Fatalf("no 1.4.1 fixture for preset %q", id)
	}
	return tokens
}

// fixture040Dark is the dark preset of launchers 0.4.0–0.9.0, an older
// generation with other colours and no --ease-spring, --dur-slow or --shadow-lift.
func fixture040Dark() map[string]string {
	tokens := fixtureLengths()
	maps.Copy(tokens, map[string]string{
		"--bg":             "#0a0f15",
		"--bg-sidebar":     "#0a0f15",
		"--surface":        "#11171f",
		"--surface-2":      "#141c25",
		"--surface-3":      "#1c242f",
		"--surface-4":      "#242c38",
		"--border":         "rgba(255, 255, 255, 0.07)",
		"--border-strong":  "rgba(255, 255, 255, 0.12)",
		"--hover":          "rgba(255, 255, 255, 0.045)",
		"--hover-strong":   "rgba(255, 255, 255, 0.075)",
		"--text":           "#eef2f6",
		"--text-2":         "#9ba6b2",
		"--text-3":         "#657180",
		"--accent":         "#6673f2",
		"--accent-hover":   "#7480f7",
		"--accent-subtle":  "rgba(102, 115, 242, 0.15)",
		"--accent-text":    "#9aa4f7",
		"--accent-ring":    "rgba(102, 115, 242, 0.5)",
		"--success":        "#65b883",
		"--success-subtle": "rgba(101, 184, 131, 0.13)",
		"--warning":        "#d8a45d",
		"--warning-subtle": "rgba(216, 164, 93, 0.13)",
		"--danger":         "#d96969",
		"--danger-subtle":  "rgba(217, 105, 105, 0.12)",
		"--font-sans":      `"Inter Tight Variable", "Inter", "Segoe UI", system-ui, sans-serif`,
		"--ease":           "cubic-bezier(0.2, 0.8, 0.2, 1)",
		"--dur":            "160ms",
		"--dur-fast":       "120ms",
		"--dur-panel":      "180ms",
		"--shadow-pop":     "0 0.8rem 2.8rem rgba(0, 0, 0, 0.45), 0 2px 0.8rem rgba(0, 0, 0, 0.3)",
		"--shadow-modal":   "0 2rem 6rem rgba(0, 0, 0, 0.55)",
	})
	return tokens
}

func withChanges(tokens, changes map[string]string) map[string]string {
	out := maps.Clone(tokens)
	maps.Copy(out, changes)
	return out
}

func oldContractFork(id, name, base string, tokens map[string]string) Theme {
	return Theme{ID: id, Name: name, Base: base, Tokens: tokens, UpdatedAt: fixtureForkTime}
}

// darkFork1x is the user's 1.x fork of the dark preset with two edits, and
// the values those edits must keep after migration.
func darkFork1x(t *testing.T, id, name string) (Theme, map[string]string) {
	t.Helper()
	tokens := withChanges(fixture141(t, "dark"), map[string]string{
		"--accent":    "#ff0055",
		"--radius-md": "1rem",
	})
	kept := map[string]string{
		"--accent":    "#ff0055",
		"--radius-md": "1.1rem",
	}
	return oldContractFork(id, name, BaseDark, tokens), kept
}

// wantAfterMigration builds the expected tokens from the parent preset as it
// is now, so the expectation follows later changes to preset values.
func wantAfterMigration(t *testing.T, parent string, stored, kept map[string]string) map[string]string {
	t.Helper()
	current, ok := presetByID(parent)
	if !ok {
		t.Fatalf("no built-in preset %q", parent)
	}
	oldContract := fixture141(t, "dark")
	want := make(map[string]string, len(allowedTokens))
	for name := range stored {
		if value, ok := kept[name]; ok {
			want[name] = value
			continue
		}
		value, ok := current.Tokens[name]
		if !ok {
			t.Fatalf("preset %s has no token %s", parent, name)
		}
		want[name] = value
	}
	for _, tok := range allowedTokens {
		if _, old := oldContract[tok.Name]; old {
			continue
		}
		value, ok := current.Tokens[tok.Name]
		if !ok {
			t.Fatalf("preset %s has no token %s", parent, tok.Name)
		}
		want[tok.Name] = value
	}
	return want
}

// tokenDiff lists every mismatch. With extrasFrom set, a token absent from
// want is accepted when it carries that preset's current value.
func tokenDiff(got, want map[string]string, extrasFrom string) []string {
	var diff []string
	for name, w := range want {
		g, ok := got[name]
		switch {
		case !ok:
			diff = append(diff, fmt.Sprintf("%s missing, want %q", name, w))
		case g != w:
			diff = append(diff, fmt.Sprintf("%s = %q, want %q", name, g, w))
		}
	}
	for name, g := range got {
		if _, ok := want[name]; ok {
			continue
		}
		if extrasFrom != "" && !isSettingsOwned(name) {
			if p, ok := presetByID(extrasFrom); ok && p.Tokens[name] == g {
				continue
			}
		}
		diff = append(diff, fmt.Sprintf("%s = %q is not expected", name, g))
	}
	slices.Sort(diff)
	return diff
}

func requireTokens(t *testing.T, what string, got, want map[string]string, extrasFrom string) {
	t.Helper()
	if diff := tokenDiff(got, want, extrasFrom); len(diff) > 0 {
		t.Errorf("%s tokens differ:\n%s", what, strings.Join(diff, "\n"))
	}
}

func jsonContract(t *testing.T, th Theme) int {
	t.Helper()
	raw, err := json.Marshal(th)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Contract int `json:"contract"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	return probe.Contract
}

func requireContract2(t *testing.T, what string, th Theme) {
	t.Helper()
	if got := jsonContract(t, th); got != 2 {
		t.Errorf("%s contract = %d, want 2", what, got)
	}
}

// stripContractField is a theme as the frontend may send it back: the same
// JSON without the contract key.
func stripContractField(t *testing.T, th Theme) Theme {
	t.Helper()
	raw, err := json.Marshal(th)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "contract")
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var out Theme
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func oldThemeText(t *testing.T, th Theme) string {
	t.Helper()
	raw, err := json.Marshal(file{Version: 1, Theme: th})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"contract"`)) {
		t.Fatalf("fixture is not an old-contract file: %s", raw)
	}
	return string(raw)
}

func writeCurrentContractThemes(t *testing.T, themes ...Theme) string {
	t.Helper()
	entries := make([]map[string]any, 0, len(themes))
	for _, th := range themes {
		raw, err := json.Marshal(th)
		if err != nil {
			t.Fatal(err)
		}
		var entry map[string]any
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatal(err)
		}
		entry["contract"] = 2
		entries = append(entries, entry)
	}
	path := filepath.Join(t.TempDir(), "themes.json")
	data := map[string]any{"themes": entries, "pending": "", "confirmed": ""}
	if err := storage.Save(path, 1, data); err != nil {
		t.Fatal(err)
	}
	return path
}

type storedThemeEntry struct {
	ID       string            `json:"id"`
	Contract int               `json:"contract"`
	Tokens   map[string]string `json:"tokens"`
}

func readThemesFile(t *testing.T, path string) (int, map[string]storedThemeEntry) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Data    struct {
			Themes []storedThemeEntry `json:"themes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("themes.json is not a state envelope: %v", err)
	}
	out := make(map[string]storedThemeEntry, len(doc.Data.Themes))
	for _, e := range doc.Data.Themes {
		out[e.ID] = e
	}
	return doc.Version, out
}

func builtInSnapshot() map[string]map[string]string {
	snap := make(map[string]map[string]string, len(presets))
	for _, p := range presets {
		snap[p.ID] = maps.Clone(p.Tokens)
	}
	return snap
}

func requireBuiltInsIntact(t *testing.T, s *Service, snap map[string]map[string]string) {
	t.Helper()
	for id, tokens := range snap {
		got, err := s.Get(id)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", id, err)
		}
		if !got.BuiltIn {
			t.Errorf("built-in %s served as a user theme", id)
		}
		requireTokens(t, "built-in "+id, got.Tokens, tokens, "")
	}
}

func TestOldContractThemeFollowsCurrentPreset(t *testing.T) {
	cases := []struct {
		name   string
		base   string
		parent string
		stored map[string]string
		kept   map[string]string
		// extras admits tokens the fixture's generation lacked, provided they
		// carry the parent's current value.
		extras bool
	}{
		{
			name:   "dark fork of 1.4 with two edits",
			base:   BaseDark,
			parent: "dark",
			stored: withChanges(fixture141(t, "dark"), map[string]string{"--accent": "#ff0055", "--radius-md": "1rem"}),
			kept:   map[string]string{"--accent": "#ff0055", "--radius-md": "1.1rem"},
		},
		{
			name:   "light fork of 1.4 with a new accent",
			base:   BaseLight,
			parent: "light",
			stored: withChanges(fixture141(t, "light"), map[string]string{"--accent": "#ff0055"}),
			kept:   map[string]string{"--accent": "#ff0055"},
		},
		{
			name:   "contrast fork of 1.4 with a softer text",
			base:   BaseDark,
			parent: "contrast",
			stored: withChanges(fixture141(t, "contrast"), map[string]string{"--text": "#fafafa"}),
			kept:   map[string]string{"--text": "#fafafa"},
		},
		{
			name:   "unchanged dark fork of 0.4",
			base:   BaseDark,
			parent: "dark",
			stored: fixture040Dark(),
			extras: true,
		},
		{
			name:   "partial map ties and goes to dark",
			base:   BaseDark,
			parent: "dark",
			stored: map[string]string{"--accent": "#ff0055", "--radius-lg": "2rem"},
			kept:   map[string]string{"--accent": "#ff0055", "--radius-lg": "2.2rem"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := builtInSnapshot()
			path := writeStoredThemes(t, []Theme{oldContractFork("fork", "Моя тема", tc.base, tc.stored)}, "")
			s := mustServiceAt(t, path)

			got, err := s.Get("fork")
			if err != nil {
				t.Fatalf("Get(fork) error = %v", err)
			}
			extrasFrom := ""
			if tc.extras {
				extrasFrom = tc.parent
			}
			requireTokens(t, "migrated fork", got.Tokens, wantAfterMigration(t, tc.parent, tc.stored, tc.kept), extrasFrom)
			requireContract2(t, "migrated fork", got)
			if got.Base != tc.base {
				t.Errorf("base = %q, want %q", got.Base, tc.base)
			}
			requireBuiltInsIntact(t, s, snap)
		})
	}
}

func TestStartupServesMigratedThemeWithoutRewritingFile(t *testing.T) {
	fork, kept := darkFork1x(t, "fork", "Моя тема")
	path := writeStoredThemes(t, []Theme{fork}, "fork")
	before, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, path)
	want := wantAfterMigration(t, "dark", fork.Tokens, kept)
	var listed Theme
	found := false
	for _, th := range s.List() {
		if th.ID == "fork" {
			listed, found = th, true
		}
	}
	if !found {
		t.Fatal("List() does not serve the stored fork")
	}
	requireTokens(t, "listed fork", listed.Tokens, want, "")
	requireContract2(t, "listed fork", listed)
	active, err := s.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	requireTokens(t, "active fork", active.Tokens, want, "")

	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("startup rewrote themes.json:\nbefore %s\nafter  %s", before, after)
	}
}

func TestMigratedThemesReachDiskOnFirstWrite(t *testing.T) {
	dark, _ := darkFork1x(t, "fork-dark", "Тёмная правка")
	light := oldContractFork("fork-light", "Светлая правка", BaseLight,
		withChanges(fixture141(t, "light"), map[string]string{"--accent": "#ff0055"}))
	path := writeStoredThemes(t, []Theme{dark, light}, "")
	s := mustServiceAt(t, path)

	served := map[string]map[string]string{}
	for _, id := range []string{"fork-dark", "fork-light"} {
		th, err := s.Get(id)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", id, err)
		}
		served[id] = maps.Clone(th.Tokens)
	}
	if got := served["fork-dark"]["--radius-md"]; got != "1.1rem" {
		t.Fatalf("served --radius-md = %q before the first write, want 1.1rem", got)
	}

	edit, err := s.Get("fork-dark")
	if err != nil {
		t.Fatal(err)
	}
	edit.Name = "Переименованная"
	if _, err := s.Save(edit); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	version, stored := readThemesFile(t, path)
	if version != 1 {
		t.Errorf("themes.json envelope version = %d, want 1", version)
	}
	for id, tokens := range served {
		entry, ok := stored[id]
		if !ok {
			t.Fatalf("themes.json lost %s after the first write", id)
		}
		if entry.Contract != 2 {
			t.Errorf("themes.json %s contract = %d, want 2", id, entry.Contract)
		}
		requireTokens(t, "persisted "+id, entry.Tokens, tokens, "")
	}

	restarted := mustServiceAt(t, path)
	for id, tokens := range served {
		th, err := restarted.Get(id)
		if err != nil {
			t.Fatalf("Get(%s) after restart error = %v", id, err)
		}
		requireTokens(t, id+" after restart", th.Tokens, tokens, "")
		requireContract2(t, id+" after restart", th)
	}
	again, err := restarted.Get("fork-dark")
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Tokens["--radius-md"]; got != "1.1rem" {
		t.Fatalf("--radius-md after restart = %q, want 1.1rem (scaled once)", got)
	}
}

func TestCurrentContractThemeIsServedAsStored(t *testing.T) {
	stored := map[string]string{
		"--bg":        "#0a0f15",
		"--radius-sm": "0.6rem",
		"--radius-md": "1rem",
		"--font-sans": "Inter Tight Variable, Inter, Segoe UI, system-ui, sans-serif",
	}
	path := writeCurrentContractThemes(t, Theme{
		ID:        "current",
		Name:      "Новая тема",
		Base:      BaseDark,
		Tokens:    maps.Clone(stored),
		UpdatedAt: fixtureForkTime,
	})
	s := mustServiceAt(t, path)
	got, err := s.Get("current")
	if err != nil {
		t.Fatalf("Get(current) error = %v", err)
	}
	requireTokens(t, "contract 2 theme", got.Tokens, stored, "")
	requireContract2(t, "contract 2 theme", got)
}

func TestOldContractImportIsMigratedAndExportedAsCurrent(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, s *Service, text string) (Theme, error)
	}{
		{"ImportText", func(t *testing.T, s *Service, text string) (Theme, error) { return s.ImportText(text, "") }},
		{"Import", func(t *testing.T, s *Service, text string) (Theme, error) {
			return s.Import(importFilePath(t, "shared.typhontheme", []byte(text)))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fork, kept := darkFork1x(t, "shared", "Из файла")
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))

			got, err := tc.run(t, s, oldThemeText(t, fork))
			if err != nil {
				t.Fatalf("import error = %v", err)
			}
			want := wantAfterMigration(t, "dark", fork.Tokens, kept)
			requireTokens(t, "imported", got.Tokens, want, "")
			requireContract2(t, "imported", got)
			stored, err := s.Get(got.ID)
			if err != nil {
				t.Fatalf("Get(%s) error = %v", got.ID, err)
			}
			requireTokens(t, "stored after import", stored.Tokens, want, "")

			out := filepath.Join(t.TempDir(), "shared.typhontheme")
			if err := s.Export(got.ID, out); err != nil {
				t.Fatalf("Export() error = %v", err)
			}
			data, err := os.ReadFile(filepath.Clean(out))
			if err != nil {
				t.Fatal(err)
			}
			var exported struct {
				Theme struct {
					Contract int               `json:"contract"`
					Tokens   map[string]string `json:"tokens"`
				} `json:"theme"`
			}
			if err := json.Unmarshal(data, &exported); err != nil {
				t.Fatalf("exported file is not a theme file: %v", err)
			}
			if exported.Theme.Contract != 2 {
				t.Errorf("exported contract = %d, want 2", exported.Theme.Contract)
			}
			requireTokens(t, "exported", exported.Theme.Tokens, want, "")

			again, err := tc.run(t, s, string(data))
			if err != nil {
				t.Fatalf("re-import of the export error = %v", err)
			}
			requireTokens(t, "re-imported export", again.Tokens, want, "")
			requireContract2(t, "re-imported export", again)
		})
	}
}

func TestWatchedOldContractFileIsMigratedOnReload(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	fork, kept := darkFork1x(t, "live", "Правка в редакторе")
	writeLater(t, w.path, oldThemeText(t, fork), time.Hour)

	w.s.pollOnce(t.Context())
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatalf("Get(live) error = %v", err)
	}
	if got.Name != "Правка в редакторе" || got.Source != filepath.Clean(w.path) {
		t.Fatalf("watched file not reloaded: name %q, source %q", got.Name, got.Source)
	}
	requireTokens(t, "reloaded", got.Tokens, wantAfterMigration(t, "dark", fork.Tokens, kept), "")
	requireContract2(t, "reloaded", got)
}

func TestSaveWithoutContractFieldDoesNotMigrateAgain(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (*Service, string, Theme)
	}{
		{
			name: "theme typed in the editor",
			setup: func(t *testing.T) (*Service, string, Theme) {
				path := filepath.Join(t.TempDir(), "themes.json")
				return mustServiceAt(t, path), path, Theme{
					ID:   "typed",
					Name: "Набранная",
					Base: BaseDark,
					Tokens: map[string]string{
						"--accent":    "#ff0055",
						"--radius-md": "1.1rem",
						"--radius-lg": "1.2rem",
					},
				}
			},
		},
		{
			name: "migrated theme back from the UI",
			setup: func(t *testing.T) (*Service, string, Theme) {
				fork, _ := darkFork1x(t, "fork", "Моя тема")
				path := writeStoredThemes(t, []Theme{fork}, "")
				s := mustServiceAt(t, path)
				served, err := s.Get("fork")
				if err != nil {
					t.Fatalf("Get(fork) error = %v", err)
				}
				if got := served.Tokens["--radius-md"]; got != "1.1rem" {
					t.Fatalf("served --radius-md = %q, want 1.1rem", got)
				}
				in := stripContractField(t, served)
				in.Name = "Переименованная"
				return s, path, in
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, path, in := tc.setup(t)
			want := maps.Clone(in.Tokens)

			saved, err := s.Save(in)
			if err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			requireTokens(t, "saved", saved.Tokens, want, "")
			requireContract2(t, "saved", saved)
			stored, err := s.Get(in.ID)
			if err != nil {
				t.Fatalf("Get(%s) error = %v", in.ID, err)
			}
			requireTokens(t, "stored", stored.Tokens, want, "")
			requireContract2(t, "stored", stored)

			_, onDisk := readThemesFile(t, path)
			entry, ok := onDisk[in.ID]
			if !ok {
				t.Fatalf("themes.json has no %s", in.ID)
			}
			if entry.Contract != 2 {
				t.Errorf("themes.json %s contract = %d, want 2", in.ID, entry.Contract)
			}
			requireTokens(t, "persisted", entry.Tokens, want, "")

			restarted, err := mustServiceAt(t, path).Get(in.ID)
			if err != nil {
				t.Fatalf("Get(%s) after restart error = %v", in.ID, err)
			}
			requireTokens(t, "after restart", restarted.Tokens, want, "")
		})
	}
}

func TestFailedFirstWriteKeepsServingMigratedThemes(t *testing.T) {
	fork, kept := darkFork1x(t, "fork", "Моя тема")
	path := writeStoredThemes(t, []Theme{fork}, "")
	s := mustServiceAt(t, path)
	before, err := s.Get("fork")
	if err != nil {
		t.Fatalf("Get(fork) error = %v", err)
	}
	requireTokens(t, "before the failed write", before.Tokens, wantAfterMigration(t, "dark", fork.Tokens, kept), "")
	beforeTokens := maps.Clone(before.Tokens)
	beforeCount := len(s.List())

	// A plain file where the state directory was: storage.Save cannot create
	// the directory past it, on any OS.
	dir := filepath.Dir(path)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(validTheme("other")); err == nil {
		t.Fatal("Save() with a blocked state directory succeeded, want error")
	}

	after, err := s.Get("fork")
	if err != nil {
		t.Fatalf("Get(fork) after failed save error = %v", err)
	}
	requireTokens(t, "after the failed write", after.Tokens, beforeTokens, "")
	requireContract2(t, "after the failed write", after)
	if n := len(s.List()); n != beforeCount {
		t.Fatalf("List() after failed save = %d themes, want %d", n, beforeCount)
	}
}

func TestUserRemLengthsKeepTheirPixelSize(t *testing.T) {
	cases := []struct {
		token  string
		stored string
		want   string
	}{
		{"--radius-xs", "1rem", "1.1rem"},
		{"--radius-sm", "0.5rem", "0.55rem"},
		{"--radius-md", ".25rem", "0.275rem"},
		{"--radius-lg", "calc(1rem + 2px)", "calc(1.1rem + 2px)"},
		{"--radius-xl", "clamp(1rem, 2vw, 3rem)", "clamp(1.1rem, 2vw, 3.3rem)"},
		{"--space-1", "0.12345rem", "0.1358rem"},
		{"--space-2", "0.7rem", "0.77rem"},
		{"--space-3", "3rem", "3.3rem"},
		{"--tracking-title", "-0.05rem", "-0.055rem"},
		{"--cut", "12px", "12px"},
		{"--space-4", "1.5em", "1.5em"},
		{"--page-max", "50%", "50%"},
		{"--font-sans", "Brand2rem Sans, sans-serif", "Brand2rem Sans, sans-serif"},
	}
	tokens := make(map[string]string, len(cases))
	for _, tc := range cases {
		kind := allowedTokenSet[tc.token]
		if !validTokenKind(tc.token, kind, tc.stored) || !validTokenKind(tc.token, kind, tc.want) {
			t.Fatalf("fixture %s: %q or %q is not a valid %s", tc.token, tc.stored, tc.want, kind)
		}
		tokens[tc.token] = tc.stored
	}
	th := oldContractFork("units", "Единицы", BaseDark, tokens)
	th.CSS = ".card { padding: 1rem; }"
	s := mustServiceAt(t, writeStoredThemes(t, []Theme{th}, ""))

	got, err := s.Get("units")
	if err != nil {
		t.Fatalf("Get(units) error = %v", err)
	}
	for _, tc := range cases {
		if v := got.Tokens[tc.token]; v != tc.want {
			t.Errorf("%s: stored %q served as %q, want %q", tc.token, tc.stored, v, tc.want)
		}
	}
	if got.CSS != th.CSS {
		t.Errorf("css = %q, want %q untouched", got.CSS, th.CSS)
	}
	requireContract2(t, "units", got)
}
