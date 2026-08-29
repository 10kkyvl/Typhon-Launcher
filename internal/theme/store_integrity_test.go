package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := NewServiceAt(""); err == nil {
		t.Fatal("empty path must not produce a service")
	}
}

func TestCorruptThemesFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `{"version":1,"data":{"themes":[`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
		{"foreign envelope version", `{"version":7,"data":{"themes":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "themes.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(path); err == nil {
				t.Fatal("corrupt themes file must not produce a service")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingThemesFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "themes.json")
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	list := s.List()
	if len(list) != len(presets) {
		t.Fatalf("List() = %d themes, want %d built-ins", len(list), len(presets))
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("themes.json created on empty startup without any mutation")
	}
}
