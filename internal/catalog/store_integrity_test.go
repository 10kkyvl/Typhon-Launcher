package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyDir(t *testing.T) {
	if _, err := NewServiceAt(""); err == nil {
		t.Fatal("empty dir must not produce a service")
	}
}

func TestCorruptStateFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{"catalog", "catalog.json"},
		{"match overrides", "match_overrides.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.file)
			const raw = `{"version":1,"data":[{"id":`
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(dir); err == nil {
				t.Fatal("corrupt state must not produce a service")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingStateStartsEmptyAndSaves(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	if len(s.ListGames()) != 0 {
		t.Fatalf("games = %+v, want none", s.ListGames())
	}
	if _, err := s.AddGame(Game{Title: "Cyberpunk 2077"}); err != nil {
		t.Fatalf("add game: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
		t.Fatalf("catalog not saved: %v", err)
	}
}
