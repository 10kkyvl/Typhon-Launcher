package theme

import (
	"maps"
	"testing"
)

// fixture030Light is the light preset of launchers 0.3.0–0.3.1: the first
// palette, smaller large radii and no --ease-spring, --dur-slow or
// --shadow-lift.
func fixture030Light() map[string]string {
	tokens := fixtureLengths()
	maps.Copy(tokens, map[string]string{
		"--radius-lg":      "1rem",
		"--radius-xl":      "1.2rem",
		"--bg":             "#f5f6f8",
		"--bg-sidebar":     "#eef0f3",
		"--surface":        "#ffffff",
		"--surface-2":      "#f7f8fa",
		"--surface-3":      "#eef0f3",
		"--surface-4":      "#e4e7ec",
		"--border":         "rgba(15, 23, 32, 0.08)",
		"--border-strong":  "rgba(15, 23, 32, 0.14)",
		"--hover":          "rgba(15, 23, 32, 0.04)",
		"--hover-strong":   "rgba(15, 23, 32, 0.07)",
		"--text":           "#10161d",
		"--text-2":         "#4b5563",
		"--text-3":         "#7a8494",
		"--accent":         "#4f5bd5",
		"--accent-hover":   "#414dc4",
		"--accent-subtle":  "rgba(79, 91, 213, 0.1)",
		"--accent-text":    "#3c46b0",
		"--accent-ring":    "rgba(79, 91, 213, 0.45)",
		"--success":        "#2f8f5b",
		"--success-subtle": "rgba(47, 143, 91, 0.12)",
		"--warning":        "#b3791f",
		"--warning-subtle": "rgba(179, 121, 31, 0.12)",
		"--danger":         "#c53c3c",
		"--danger-subtle":  "rgba(197, 60, 60, 0.1)",
		"--font-sans":      `"Inter Tight Variable", "Inter", "Segoe UI", system-ui, sans-serif`,
		"--ease":           "cubic-bezier(0.2, 0.8, 0.2, 1)",
		"--dur":            "160ms",
		"--dur-fast":       "120ms",
		"--dur-panel":      "180ms",
		"--shadow-pop":     "0 0.8rem 2.8rem rgba(15, 23, 32, 0.12), 0 2px 0.8rem rgba(15, 23, 32, 0.08)",
		"--shadow-modal":   "0 2rem 6rem rgba(15, 23, 32, 0.16)",
	})
	return tokens
}

// migratedFrom loads one old-contract theme through startup and returns what
// the service serves for it.
func migratedFrom(t *testing.T, base string, tokens map[string]string) Theme {
	t.Helper()
	s := mustServiceAt(t, writeStoredThemes(t, []Theme{oldContractFork("fork", "Моя тема", base, tokens)}, ""))
	got, err := s.Get("fork")
	if err != nil {
		t.Fatalf("Get(fork) error = %v", err)
	}
	return got
}

// currentWith is the full current token set of a preset with the user's
// values on top.
func currentWith(t *testing.T, parent string, kept map[string]string) map[string]string {
	t.Helper()
	p, ok := presetByID(parent)
	if !ok {
		t.Fatalf("no built-in preset %q", parent)
	}
	want := make(map[string]string, len(allowedTokens))
	for _, tok := range allowedTokens {
		value, ok := p.Tokens[tok.Name]
		if !ok {
			t.Fatalf("preset %s has no token %s", parent, tok.Name)
		}
		want[tok.Name] = value
	}
	maps.Copy(want, kept)
	return want
}

func TestUserShadowWithRemKeepsItsPixelSize(t *testing.T) {
	got := migratedFrom(t, BaseDark, withChanges(fixture141(t, "dark"), map[string]string{
		"--shadow-modal": "0 3rem 5rem rgba(0, 0, 0, 0.7)",
		"--shadow-lift":  "0 1rem 2rem -0.5rem rgba(0, 0, 0, 0.6), inset 0 1px 0 #ffffff",
	}))
	requireTokens(t, "migrated fork", got.Tokens, currentWith(t, "dark", map[string]string{
		"--shadow-modal": "0 3.3rem 5.5rem rgba(0, 0, 0, 0.7)",
		"--shadow-lift":  "0 1.1rem 2.2rem -0.55rem rgba(0, 0, 0, 0.6), inset 0 1px 0 #ffffff",
	}), "")
}

func TestOldestGenerationForkFollowsCurrentPreset(t *testing.T) {
	got := migratedFrom(t, BaseLight, withChanges(fixture030Light(), map[string]string{"--accent": "#ff0055"}))
	requireTokens(t, "migrated 0.3 fork", got.Tokens, currentWith(t, "light", map[string]string{"--accent": "#ff0055"}), "")
	requireContract2(t, "migrated 0.3 fork", got)
}

func TestFull040ForkGainsTheTokensItLacked(t *testing.T) {
	got := migratedFrom(t, BaseDark, fixture040Dark())
	requireTokens(t, "migrated 0.4 fork", got.Tokens, currentWith(t, "dark", nil), "")
}

// --radius-lg 1rem is the value the 0.3 presets shipped; in a 1.4 fork it is
// the user's own edit and must not pull the fork into the older generation.
func TestValueOfAnOlderGenerationIsAUserEdit(t *testing.T) {
	got := migratedFrom(t, BaseDark, withChanges(fixture141(t, "dark"), map[string]string{"--radius-lg": "1rem"}))
	requireTokens(t, "migrated fork", got.Tokens, currentWith(t, "dark", map[string]string{"--radius-lg": "1.1rem"}), "")
}

// A rem value near the length limit would leave the allowed range once
// scaled; it is kept as stored so the theme can still be saved.
func TestScaledValuePastTheLimitIsKeptAsStored(t *testing.T) {
	got := migratedFrom(t, BaseDark, map[string]string{"--page-max": "950rem", "--space-12": "5rem"})
	if v := got.Tokens["--page-max"]; v != "950rem" {
		t.Errorf("--page-max = %q, want 950rem kept", v)
	}
	if v := got.Tokens["--space-12"]; v != "5.5rem" {
		t.Errorf("--space-12 = %q, want 5.5rem", v)
	}
}

func TestMigratedForksPassValidationAndSave(t *testing.T) {
	units := map[string]string{
		"--radius-md":      ".25rem",
		"--radius-lg":      "calc(1rem + 2px)",
		"--radius-xl":      "clamp(1rem, 2vw, 3rem)",
		"--space-1":        "0.12345rem",
		"--tracking-title": "-0.05rem",
		"--font-sans":      "Brand2rem Sans, sans-serif",
	}
	cases := []struct {
		name   string
		base   string
		tokens map[string]string
	}{
		{"dark 1.4 with edits", BaseDark, withChanges(fixture141(t, "dark"), map[string]string{"--accent": "#ff0055", "--radius-md": "1rem"})},
		{"light 1.4 with edits", BaseLight, withChanges(fixture141(t, "light"), map[string]string{"--accent": "#ff0055"})},
		{"contrast 1.4 with edits", BaseDark, withChanges(fixture141(t, "contrast"), map[string]string{"--text": "#fafafa"})},
		{"dark 0.4 with its quoted font", BaseDark, fixture040Dark()},
		{"light 0.3 with its quoted font", BaseLight, fixture030Light()},
		{"partial map of rem edits", BaseDark, units},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeStoredThemes(t, []Theme{oldContractFork("fork", "Моя тема", tc.base, tc.tokens)}, "")
			s := mustServiceAt(t, path)
			got, err := s.Get("fork")
			if err != nil {
				t.Fatalf("Get(fork) error = %v", err)
			}
			if _, err := Validate(got); err != nil {
				t.Fatalf("migrated theme refused by Validate: %v", err)
			}
			if _, err := s.Save(stripContractField(t, got)); err != nil {
				t.Fatalf("editor save of the migrated theme error = %v", err)
			}
		})
	}
}
