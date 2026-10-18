package watchlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyPath(t *testing.T) {
	if _, err := NewServiceAt("", &fakeReleases{}, fakeTitles{}); err == nil {
		t.Fatal("empty path must not produce a service")
	}
}

func TestCorruptWatchlistFailsStartupAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `{"entries":[{"gameId":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
		{"unsupported version", `{"version":99,"data":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "watchlist.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceAt(path, &fakeReleases{}, fakeTitles{"g1": "G"}); err == nil {
				t.Fatal("corrupt watchlist must not produce a service")
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

func TestMissingWatchlistStartsEmptyAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchlist.json")
	s, err := NewServiceAt(path, &fakeReleases{}, fakeTitles{"g1": "Game"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("entries = %+v, want none", got)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("constructor must not create the file")
	}
	if _, err := s.Watch("g1"); err != nil {
		t.Fatalf("watch: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("watchlist not saved: %v", err)
	}
}
