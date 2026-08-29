package lan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := NewServiceAt("", nil, nil); err == nil {
		t.Fatal("empty config dir must not produce a service")
	}
}

func TestCorruptLanSharesFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"gameId":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, "lanshares.json")
		if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewServiceAt(dir, nil, nil); err == nil {
			t.Errorf("%s: corrupt lanshares.json must not produce a service", tc.name)
		}
		got, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.raw {
			t.Errorf("%s: file rewritten: %q", tc.name, got)
		}
	}
}

func TestMissingLanSharesStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServiceAt(dir, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if got := s.Shares(); len(got) != 0 {
		t.Fatalf("Shares() = %+v, want none", got)
	}
}
