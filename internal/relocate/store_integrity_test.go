package relocate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := NewServiceAt("", nil, nil, nil, nil, nil); err == nil {
		t.Fatal("empty dir must not produce a service")
	}
}

func TestCorruptJournalFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"id":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, journalName)
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(dir, nil, nil, nil, nil, nil); err == nil {
				t.Fatal("corrupt journal must not produce a service")
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

func TestNewServiceAtMissingJournalStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServiceAt(dir, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("jobs = %+v, want none", got)
	}
}
