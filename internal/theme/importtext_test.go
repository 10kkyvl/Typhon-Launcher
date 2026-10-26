package theme

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func themeText(id, name string) string {
	return `{"version":1,"theme":{"id":"` + id + `","name":"` + name + `","base":"dark","tokens":{"--bg":"#101010"},"css":""}}`
}

func TestImportText(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		override string
		wantName string
		wantErr  error
	}{
		{name: "happy path", text: themeText("pasted", "Из текста"), wantName: "Из текста"},
		{name: "name override", text: themeText("pasted", "Из текста"), override: "  Своя  ", wantName: "Своя"},
		{name: "blank override keeps file name", text: themeText("pasted", "Из текста"), override: "   ", wantName: "Из текста"},
		{name: "invalid name override", text: themeText("pasted", "Из текста"), override: strings.Repeat("я", 65), wantErr: ErrThemeName},
		{name: "control char in override", text: themeText("pasted", "Из текста"), override: "a\x01b", wantErr: ErrThemeName},
		{name: "empty text", text: "", wantErr: ErrThemeFormat},
		{name: "whitespace text", text: " \n\t ", wantErr: ErrThemeFormat},
		{name: "garbage", text: "это не тема", wantErr: ErrThemeFormat},
		{name: "truncated json", text: `{"version":1,"theme":{"id":"x"`, wantErr: ErrThemeFormat},
		{name: "unknown field", text: `{"version":1,"theme":{"id":"x","name":"X","base":"dark","tokens":{},"extra":1}}`, wantErr: ErrThemeFormat},
		{name: "wrong version", text: `{"version":2,"theme":{"id":"x","name":"X","base":"dark","tokens":{}}}`, wantErr: ErrThemeVersion},
		{name: "oversize", text: strings.Repeat(" ", MaxThemeBytes+1), wantErr: ErrThemeSize},
		{name: "invalid token", text: `{"version":1,"theme":{"id":"x","name":"X","base":"dark","tokens":{"--bg":"url(x)"}}}`, wantErr: ErrThemeTokenValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			got, err := s.ImportText(tc.text, tc.override)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ImportText() error = %v, want %v", err, tc.wantErr)
				}
				if len(s.List()) != len(presets) {
					t.Fatalf("failed import stored a theme: %d themes", len(s.List()))
				}
				return
			}
			if err != nil {
				t.Fatalf("ImportText() error = %v", err)
			}
			if got.Name != tc.wantName || got.ID != "pasted" || got.Source != "" || got.BuiltIn {
				t.Fatalf("ImportText() = %+v, want name %q, id pasted, no source", got, tc.wantName)
			}
			stored, err := s.Get("pasted")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if stored.Name != tc.wantName {
				t.Fatalf("stored name = %q, want %q", stored.Name, tc.wantName)
			}
		})
	}
}

func TestImportTextFormatErrorCarriesCode(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	_, err := s.ImportText("{", "")
	if code := uierr.Code(err); code != "theme.invalid_format" {
		t.Fatalf("uierr.Code(%v) = %q, want theme.invalid_format", err, code)
	}
}

func TestImportTextSameID(t *testing.T) {
	cases := []struct {
		name       string
		textName   string
		override   string
		wantNewID  bool
		wantStored string
	}{
		{name: "different name forks", textName: "Из текста", override: "Другая", wantNewID: true, wantStored: "Моя тема"},
		{name: "same name overwrites", textName: "Из текста", override: "Моя тема", wantStored: "Моя тема"},
		{name: "no override, another name in the text forks", textName: "Из текста", wantNewID: true, wantStored: "Моя тема"},
		{name: "no override, same name in the text overwrites", textName: "Моя тема", wantStored: "Моя тема"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			if _, err := s.Save(validTheme("mine")); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := s.ImportText(themeText("mine", tc.textName), tc.override)
			if err != nil {
				t.Fatalf("ImportText() error = %v", err)
			}
			if (got.ID != "mine") != tc.wantNewID {
				t.Fatalf("ImportText() id = %q, want new id: %v", got.ID, tc.wantNewID)
			}
			stored, err := s.Get("mine")
			if err != nil {
				t.Fatalf("Get(mine) error = %v", err)
			}
			if stored.Name != tc.wantStored {
				t.Fatalf("theme mine name = %q, want %q", stored.Name, tc.wantStored)
			}
		})
	}
}

func TestImportFileGarbageIsFormatError(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	path := importFilePath(t, "garbage.typhontheme", []byte("not json"))
	if _, err := s.Import(path); !errors.Is(err, ErrThemeFormat) {
		t.Fatalf("Import() error = %v, want ErrThemeFormat", err)
	}
}

func TestImportSetsSourceAndExportOmitsIt(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "themes.json")
	s := mustServiceAt(t, statePath)
	path := importFilePath(t, "file.typhontheme", []byte(themeText("from-file", "Файл")))

	got, err := s.Import(path)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if got.Source != filepath.Clean(path) {
		t.Fatalf("Import() source = %q, want %q", got.Source, filepath.Clean(path))
	}

	reloaded := mustServiceAt(t, statePath)
	stored, err := reloaded.Get("from-file")
	if err != nil {
		t.Fatalf("Get() after restart error = %v", err)
	}
	if stored.Source != filepath.Clean(path) {
		t.Fatalf("source after restart = %q, want %q", stored.Source, filepath.Clean(path))
	}

	out := filepath.Join(t.TempDir(), "out.typhontheme")
	if err := s.Export("from-file", out); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"source"`) {
		t.Fatalf("exported file carries the source path: %s", data)
	}
}

func TestSaveClearsSource(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, s *Service) Theme
	}{
		{
			name: "edited imported theme",
			setup: func(t *testing.T, s *Service) Theme {
				path := importFilePath(t, "file.typhontheme", []byte(themeText("from-file", "Файл")))
				got, err := s.Import(path)
				if err != nil {
					t.Fatalf("Import() error = %v", err)
				}
				got.Name = "Правка"
				return got
			},
		},
		{
			name: "source from caller",
			setup: func(t *testing.T, _ *Service) Theme {
				tm := validTheme("from-file")
				tm.Source = filepath.Join(t.TempDir(), "secret.json")
				return tm
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			saved, err := s.Save(tc.setup(t, s))
			if err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			stored, err := s.Get("from-file")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if saved.Source != "" || stored.Source != "" {
				t.Fatalf("Save() kept source: returned %q, stored %q", saved.Source, stored.Source)
			}
		})
	}
}
