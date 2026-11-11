package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var partsForkTokens = map[string]string{"--accent": "#ff0055"}

func requireAcceptedCSS(t *testing.T, css string) {
	t.Helper()
	if err := validateCSS(css); err != nil {
		t.Fatalf("fixture css refused by the current rules: %v\n%s", err, css)
	}
}

// servedCSS loads one theme through startup and returns the CSS served for it.
func servedCSS(t *testing.T, th Theme) Theme {
	t.Helper()
	s := mustServiceAt(t, writeStoredThemes(t, []Theme{th}, ""))
	got, err := s.Get(th.ID)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", th.ID, err)
	}
	return got
}

func oldForkWithCSS(css string) Theme {
	th := oldContractFork("fork", "Моя тема", BaseDark, partsForkTokens)
	th.CSS = css
	return th
}

func storedCSS(t *testing.T, path, id string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Data struct {
			Themes []struct {
				ID  string `json:"id"`
				CSS string `json:"css"`
			} `json:"themes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("themes.json is not a state envelope: %v", err)
	}
	for _, th := range doc.Data.Themes {
		if th.ID == id {
			return th.CSS
		}
	}
	t.Fatalf("themes.json has no %s", id)
	return ""
}

func TestOldPartSelectorsAreRenamed(t *testing.T) {
	cases := []struct {
		name string
		css  string
		want string
	}{
		{"double quotes", `[data-part="sidebar-account"] { opacity: 0.9; }`, `[data-part="topbar-account"] { opacity: 0.9; }`},
		{"single quotes", `[data-part='sidebar-account'] { opacity: 0.9; }`, `[data-part='topbar-account'] { opacity: 0.9; }`},
		{"unquoted", `[data-part=sidebar-account] { opacity: 0.9; }`, `[data-part=topbar-account] { opacity: 0.9; }`},
		{"spaces inside the brackets", `[ data-part = "sidebar-account" ] { opacity: 0.9; }`, `[ data-part = "topbar-account" ] { opacity: 0.9; }`},
		{"upper-case attribute", `[DATA-PART="sidebar-account"] { opacity: 0.9; }`, `[DATA-PART="topbar-account"] { opacity: 0.9; }`},
		{"case-insensitive flag", `[data-part="Sidebar-Account" i] { opacity: 0.9; }`, `[data-part="topbar-account" i] { opacity: 0.9; }`},
		{"word operator", `[data-part~="sidebar-account"] { opacity: 0.9; }`, `[data-part~="topbar-account"] { opacity: 0.9; }`},
		{"dash operator", `[data-part|=sidebar-account] { opacity: 0.9; }`, `[data-part|=topbar-account] { opacity: 0.9; }`},
		{"prefix operator, whole name", `[data-part^="sidebar-account"] { opacity: 0.9; }`, `[data-part^="topbar-account"] { opacity: 0.9; }`},
		{"suffix operator, whole name", `[data-part$='sidebar-account'] { opacity: 0.9; }`, `[data-part$='topbar-account'] { opacity: 0.9; }`},
		{"substring operator, whole name", `[data-part*=sidebar-account] { opacity: 0.9; }`, `[data-part*=topbar-account] { opacity: 0.9; }`},
		{
			"compound, nested and repeated",
			"@media (max-width: 900px) {\n  .x:is([data-part=\"sidebar-account\"], .y) [data-part=\"sidebar-item\"]:hover { gap: 1rem; }\n}\n[data-part='sidebar-account']::after { content: \"\"; }",
			"@media (max-width: 900px) {\n  .x:is([data-part=\"topbar-account\"], .y) [data-part=\"sidebar-item\"]:hover { gap: 1rem; }\n}\n[data-part='topbar-account']::after { content: \"\"; }",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireAcceptedCSS(t, tc.css)
			got := servedCSS(t, oldForkWithCSS(tc.css))
			if got.CSS != tc.want {
				t.Errorf("css = %q, want %q", got.CSS, tc.want)
			}
			if got.CSSBlocked {
				t.Error("converted css is blocked")
			}
			requireContract2(t, "converted theme", got)
		})
	}
}

// The name is rewritten only as the value of a data-part attribute selector.
func TestPartNameLookAlikesStay(t *testing.T) {
	cases := []struct {
		name string
		css  string
	}{
		{"class", `.sidebar-account-x { opacity: 0.9; }`},
		{"id", `#sidebar-account { opacity: 0.9; }`},
		{"comment", `/* [data-part="sidebar-account"] moved */ .a { opacity: 0.9; }`},
		{"content string", `.a::after { content: "[data-part=sidebar-account]"; }`},
		{"single-quoted string", `.a::after { content: '[data-part="sidebar-account"]'; }`},
		{"longer value", `[data-part="sidebar-account-extra"] { opacity: 0.9; }`},
		{"part of the name", `[data-part^="sidebar-acc"] { opacity: 0.9; }`},
		{"other attribute", `[data-id="sidebar-account"] { opacity: 0.9; }`},
		{"longer attribute", `[data-parts="sidebar-account"] { opacity: 0.9; }`},
		{"case differs without the flag", `[data-part="Sidebar-Account"] { opacity: 0.9; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireAcceptedCSS(t, tc.css)
			if got := servedCSS(t, oldForkWithCSS(tc.css)); got.CSS != tc.css {
				t.Errorf("css = %q, want %q untouched", got.CSS, tc.css)
			}
		})
	}
}

func TestPartRenameLeavesCurrentThemesAlone(t *testing.T) {
	const css = `[data-part="sidebar-account"] { opacity: 0.9; }`
	t.Run("contract 2", func(t *testing.T) {
		th := Theme{ID: "current", Name: "Новая", Base: BaseDark, Tokens: map[string]string{"--accent": "#ff0055"}, CSS: css, UpdatedAt: fixtureForkTime}
		s := mustServiceAt(t, writeCurrentContractThemes(t, th))
		got, err := s.Get("current")
		if err != nil {
			t.Fatalf("Get(current) error = %v", err)
		}
		if got.CSS != css {
			t.Errorf("css = %q, want %q untouched", got.CSS, css)
		}
	})
	t.Run("lost marker", func(t *testing.T) {
		th := oldContractFork("fork", "Моя тема", BaseDark, map[string]string{"--accent": "#ff0055", "--font-mono": "Iosevka, monospace"})
		th.CSS = css
		got := servedCSS(t, th)
		if got.CSS != css {
			t.Errorf("css = %q, want %q untouched", got.CSS, css)
		}
		requireContract2(t, "stamped theme", got)
	})
}

func TestImportedOldPartSelectorsAreRenamed(t *testing.T) {
	const css = `[data-part="sidebar-account"] { opacity: 0.9; }`
	const want = `[data-part="topbar-account"] { opacity: 0.9; }`
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
			th := oldForkWithCSS(css)
			th.ID = "shared"
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			got, err := tc.run(t, s, oldThemeText(t, th))
			if err != nil {
				t.Fatalf("import error = %v", err)
			}
			if got.CSS != want {
				t.Errorf("css = %q, want %q", got.CSS, want)
			}
		})
	}
	t.Run("watched reload", func(t *testing.T) {
		w := newWatched(t)
		w.s.pollOnce(t.Context())
		th := oldForkWithCSS(css)
		th.ID, th.Name = "live", "Правка в редакторе"
		writeLater(t, w.path, oldThemeText(t, th), time.Hour)
		w.s.pollOnce(t.Context())
		got, err := w.s.Get("live")
		if err != nil {
			t.Fatalf("Get(live) error = %v", err)
		}
		if got.Name != th.Name {
			t.Fatalf("watched file not reloaded: name %q", got.Name)
		}
		if got.CSS != want {
			t.Errorf("css = %q, want %q", got.CSS, want)
		}
	})
}

func TestRenamedPartSelectorsReachDiskOnce(t *testing.T) {
	const want = `[data-part="topbar-account"] { opacity: 0.9; }`
	path := writeStoredThemes(t, []Theme{oldForkWithCSS(`[data-part="sidebar-account"] { opacity: 0.9; }`)}, "")
	s := mustServiceAt(t, path)
	if _, err := s.Save(validTheme("other")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := storedCSS(t, path, "fork"); got != want {
		t.Errorf("stored css = %q, want %q", got, want)
	}
	for round := 1; round <= 2; round++ {
		got, err := mustServiceAt(t, path).Get("fork")
		if err != nil {
			t.Fatalf("round %d: Get(fork) error = %v", round, err)
		}
		if got.CSS != want {
			t.Errorf("round %d: css = %q, want %q", round, got.CSS, want)
		}
	}
}

// CSS the current rules refuse keeps its stored text and is served blocked,
// as any refused CSS is.
func TestRefusedOldCSSIsBlockedAndKeptAsStored(t *testing.T) {
	const css = `@import "https://evil.test/x.css"; [data-part="sidebar-account"] { opacity: 0.9; }`
	if err := validateCSS(css); err == nil {
		t.Fatal("fixture css is accepted by the current rules")
	}
	path := writeStoredThemes(t, []Theme{oldForkWithCSS(css)}, "")
	s := mustServiceAt(t, path)
	got, err := s.Get("fork")
	if err != nil {
		t.Fatalf("Get(fork) error = %v", err)
	}
	if got.CSS != "" || !got.CSSBlocked {
		t.Errorf("served css = %q blocked %v, want empty and blocked", got.CSS, got.CSSBlocked)
	}
	if _, err := s.Save(validTheme("other")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if stored := storedCSS(t, path, "fork"); stored != css {
		t.Errorf("stored css = %q, want %q kept", stored, css)
	}
}

// Part names are public API for themes: a part of the last 1.x release that
// is gone from layout.json must be listed in renamedParts.
func TestRenamedPartsFollowLayoutJSON(t *testing.T) {
	if layoutSpecErr != nil {
		t.Fatalf("layout.json: %v", layoutSpecErr)
	}
	current := layoutIDs.Parts
	for old, now := range renamedParts {
		if !slices.Contains(current, now) {
			t.Errorf("renamedParts maps %q to %q, which layout.json does not list", old, now)
		}
		if slices.Contains(current, old) {
			t.Errorf("renamedParts renames %q, which layout.json still lists", old)
		}
		if !slices.Contains(releasedLayoutParts, old) {
			t.Errorf("renamedParts renames %q, which no release offered", old)
		}
	}
	var missing []string
	for _, part := range releasedLayoutParts {
		if _, renamed := renamedParts[part]; !slices.Contains(current, part) && !renamed {
			missing = append(missing, part)
		}
	}
	if len(missing) > 0 {
		t.Errorf("parts %s of the last release are gone from layout.json without an entry in renamedParts (contract.go): 1.x theme CSS selecting them would stop matching",
			strings.Join(missing, ", "))
	}
}
