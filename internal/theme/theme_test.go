package theme

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustServiceAt(t testing.TB, path string) *Service {
	t.Helper()
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new theme service at %s: %v", path, err)
	}
	return s
}

func validTheme(id string) Theme {
	return Theme{
		ID:   id,
		Name: "Моя тема",
		Base: BaseDark,
		Tokens: map[string]string{
			"--bg":   "#101010",
			"--dur":  "160ms",
			"--ease": "cubic-bezier(0.2, 0.8, 0.2, 1)",
		},
		CSS: ".panel { color: red; }",
	}
}

func TestValidateTokenValue(t *testing.T) {
	valid := []string{
		"#0b0f14",
		"rgba(255,255,255,0.06)",
		"1.2rem",
		"clamp(3.6rem, 2.6vw, 4.4rem)",
		"cubic-bezier(0.2, 0.8, 0.2, 1)",
		"160ms",
	}
	for _, v := range valid {
		if err := validateTokenValue(v); err != nil {
			t.Errorf("validateTokenValue(%q) = %v, want nil", v, err)
		}
	}

	invalid := map[string]string{
		"":                         "empty",
		strings.Repeat("a", 121):   "too long",
		"red;color:blue":           "semicolon",
		"url(javascript:alert(1))": "url(",
		"/*hack*/red":              "comment marker",
		"#fff\\":                   "backslash",
		"@media":                   "at-rule",
	}
	for v, why := range invalid {
		if err := validateTokenValue(v); !errors.Is(err, ErrThemeTokenValue) {
			t.Errorf("validateTokenValue(%q) [%s] = %v, want ErrThemeTokenValue", v, why, err)
		}
	}
}

func TestSaveRejectsBuiltIn(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	tm := validTheme("mine")
	tm.BuiltIn = true
	if _, err := s.Save(tm); !errors.Is(err, ErrThemeBuiltIn) {
		t.Fatalf("Save() error = %v, want ErrThemeBuiltIn", err)
	}
}

func TestDeleteRejectsBuiltIn(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	if err := s.Delete("dark"); !errors.Is(err, ErrThemeBuiltIn) {
		t.Fatalf("Delete() error = %v, want ErrThemeBuiltIn", err)
	}
}

func TestSaveCollidingWithPresetGetsNewID(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	tm := validTheme("dark")
	saved, err := s.Save(tm)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ID == "dark" {
		t.Fatalf("Save() kept preset id %q, want a forked id", saved.ID)
	}
	if preset, err := s.Get("dark"); err != nil || !preset.BuiltIn {
		t.Fatalf("preset dark was overwritten: %+v, err=%v", preset, err)
	}
}

func TestSaveRollsBackOnPersistFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	path := filepath.Join(blocker, "themes.json")
	s := mustServiceAt(t, path)

	before := s.List()

	// "blocker" must hold the state directory, but a plain file sits there
	// instead: storage.Save's os.MkdirAll cannot heal past it, so the
	// persist deterministically fails without depending on OS ACL support.
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Save(validTheme("mine")); err == nil {
		t.Fatal("Save() with a blocked state directory succeeded, want error")
	}

	after := s.List()
	if len(after) != len(before) {
		t.Fatalf("List() after failed save = %d themes, want %d (unchanged)", len(after), len(before))
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	saved, err := s.Save(validTheme("mine"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "exported.typhontheme")
	if err := s.Export(saved.ID, exportPath); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	imported, err := s.Import(exportPath)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if imported.ID != saved.ID {
		t.Fatalf("Import() id = %q, want %q", imported.ID, saved.ID)
	}
	if imported.Name != saved.Name || imported.Base != saved.Base || imported.CSS != saved.CSS {
		t.Fatalf("Import() = %+v, want fields matching %+v", imported, saved)
	}
	if len(imported.Tokens) != len(saved.Tokens) {
		t.Fatalf("Import() tokens = %+v, want %+v", imported.Tokens, saved.Tokens)
	}
	for k, v := range saved.Tokens {
		if imported.Tokens[k] != v {
			t.Fatalf("Import() token %s = %q, want %q", k, imported.Tokens[k], v)
		}
	}
}

func importFilePath(t *testing.T, name string, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportRejects(t *testing.T) {
	nested5 := ".a{.b{.c{.d{.e{color:red}}}}}"

	cases := []struct {
		name     string
		fileName string
		body     []byte
		relative bool
		missing  bool
		wantErr  error
	}{
		{
			name:     "file too large",
			fileName: "big.json",
			body:     []byte(strings.Repeat("a", 300*1024)),
			wantErr:  ErrThemeSize,
		},
		{
			name:     "unsupported version",
			fileName: "v2.json",
			body:     []byte(`{"version":2,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":""}}`),
			wantErr:  ErrThemeVersion,
		},
		{
			name:     "id path traversal",
			fileName: "traverse.json",
			body:     []byte(`{"version":1,"theme":{"id":"../evil","name":"Custom","base":"dark","tokens":{},"css":""}}`),
			wantErr:  ErrThemeID,
		},
		{
			name:     "id uppercase",
			fileName: "upper.json",
			body:     []byte(`{"version":1,"theme":{"id":"Custom","name":"Custom","base":"dark","tokens":{},"css":""}}`),
			wantErr:  ErrThemeID,
		},
		{
			name:     "unknown token name",
			fileName: "unknown-token.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{"--not-a-real-token":"#111111"},"css":""}}`),
			wantErr:  ErrThemeTokenName,
		},
		{
			name:     "semicolon in token value",
			fileName: "semicolon.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{"--bg":"#111111;color:red"},"css":""}}`),
			wantErr:  ErrThemeTokenValue,
		},
		{
			name:     "url( in token value",
			fileName: "url-token.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{"--bg":"url(evil)"},"css":""}}`),
			wantErr:  ErrThemeTokenValue,
		},
		{
			name:     "css @import",
			fileName: "css-import.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":"@import url(x.css);"}}`),
			wantErr:  ErrThemeCSSContent,
		},
		{
			name:     "css url(",
			fileName: "css-url.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":".a { background: url(x.png); }"}}`),
			wantErr:  ErrThemeCSSContent,
		},
		{
			name:     "css closing style tag",
			fileName: "css-style.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":"</style><script>1</script>"}}`),
			wantErr:  ErrThemeCSSContent,
		},
		{
			name:     "unbalanced braces",
			fileName: "css-brace.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":".a { color: red;"}}`),
			wantErr:  ErrThemeCSSBraces,
		},
		{
			name:     "nesting depth 5",
			fileName: "css-depth.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":"` + nested5 + `"}}`),
			wantErr:  ErrThemeCSSBraces,
		},
		{
			name:     "unknown json field",
			fileName: "extra-field.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":"","extra":"x"}}`),
			wantErr:  nil,
		},
		{
			name:     "relative path",
			fileName: "custom.json",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":""}}`),
			relative: true,
			wantErr:  ErrThemePath,
		},
		{
			name:     "nonexistent file",
			fileName: "missing.json",
			missing:  true,
		},
		{
			name:     "unsupported extension",
			fileName: "custom.txt",
			body:     []byte(`{"version":1,"theme":{"id":"custom","name":"Custom","base":"dark","tokens":{},"css":""}}`),
			wantErr:  ErrThemePath,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))

			var path string
			switch {
			case tc.relative:
				path = filepath.Join("relative", tc.fileName)
			case tc.missing:
				path = filepath.Join(t.TempDir(), tc.fileName)
			default:
				path = importFilePath(t, tc.fileName, tc.body)
			}

			_, err := s.Import(path)
			if err == nil {
				t.Fatalf("Import(%s) succeeded, want error", tc.name)
			}
			switch {
			case tc.missing:
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("Import(%s) error = %v, want fs.ErrNotExist", tc.name, err)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Import(%s) error = %v, want %v", tc.name, err, tc.wantErr)
				}
			}
		})
	}
}

func TestPendingThemeRevertsOnStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "themes.json")
	s1 := mustServiceAt(t, path)

	saved, err := s1.Save(validTheme("mine"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := s1.Apply(saved.ID); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	s2, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new theme service after crash: %v", err)
	}
	active, err := s2.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active.ID != defaultTheme().ID {
		t.Fatalf("Active() after unconfirmed apply = %q, want default %q", active.ID, defaultTheme().ID)
	}

	if _, err := s2.Get(saved.ID); err != nil {
		t.Fatalf("saved theme lost across revert: %v", err)
	}

	s3, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new theme service after revert persisted: %v", err)
	}
	active3, err := s3.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active3.ID != defaultTheme().ID {
		t.Fatalf("revert did not persist: Active() = %q, want default %q", active3.ID, defaultTheme().ID)
	}
}

func TestConfirmClearsPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "themes.json")
	s := mustServiceAt(t, path)

	saved, err := s.Save(validTheme("mine"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := s.Apply(saved.ID); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := s.Confirm(saved.ID); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	active, err := s.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active.ID != saved.ID {
		t.Fatalf("Active() after confirm = %q, want %q", active.ID, saved.ID)
	}

	s2, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new theme service after confirm: %v", err)
	}
	active2, err := s2.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active2.ID != saved.ID {
		t.Fatalf("confirmed theme did not survive restart: Active() = %q, want %q", active2.ID, saved.ID)
	}
}
