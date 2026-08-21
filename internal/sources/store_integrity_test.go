package sources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyDir(t *testing.T) {
	if _, err := newServiceAt("", nil, nil); err == nil {
		t.Fatal("empty dir must not produce a service")
	}
}

func TestCorruptSourcesFailStartupAndKeepFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated envelope", `{"version":1,"data":[{"id":`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sources.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := newServiceAt(dir, nil, nil); err == nil {
				t.Fatal("corrupt sources must not produce a service")
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

func TestCorruptReleasesFailStartup(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.saveSources([]Source{{ID: "src-1", Name: "Feed", URL: "https://example.test/feed.json"}}); err != nil {
		t.Fatalf("save sources: %v", err)
	}
	path := st.releasesPath("src-1")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"data":[{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newServiceAt(dir, nil, nil); err == nil {
		t.Fatal("corrupt releases must not produce a service")
	}
}

func TestMissingSourcesStartEmptyAndSave(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, nil)
	if len(s.ListSources()) != 0 {
		t.Fatalf("sources = %+v, want none", s.ListSources())
	}
	if err := s.store.saveSources([]Source{{ID: "src-1", Name: "Feed"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); err != nil {
		t.Fatalf("sources not saved: %v", err)
	}
}
