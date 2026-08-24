package settings

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

func TestCorruptSettingsFailStartupAndKeepFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `{"theme":"dark"`},
		{"garbage", `not json at all`},
		{"array root", `[1,2,3]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(path); err == nil {
				t.Fatal("corrupt settings must not produce a service")
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

func TestMissingSettingsStartWithDefaultsAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	if s.GetSettings() != Defaults() {
		t.Fatalf("got %+v, want defaults", s.GetSettings())
	}
	if err := s.SaveSettings(s.GetSettings()); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings not saved: %v", err)
	}
}
