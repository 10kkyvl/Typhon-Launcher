package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func seedGames(t *testing.T, path string, games []Game) *Service {
	t.Helper()
	s := mustServiceAt(t, path)
	s.games = games
	if err := s.persist(); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	return s
}

func TestSyncTitles(t *testing.T) {
	cases := []struct {
		name    string
		games   []Game
		resolve func(string, string) string
		want    []string
	}{
		{
			name:    "canonical title replaces the download name",
			games:   []Game{{ID: "a", Title: "setup drive rally 1.5 1 (93251)", CanonicalGameID: "canon-1"}},
			resolve: func(string, string) string { return "#DRIVE Rally" },
			want:    []string{"#DRIVE Rally"},
		},
		{
			name:    "entry without provenance is left alone",
			games:   []Game{{ID: "a", Title: "Local Game"}},
			resolve: func(string, string) string { return "#DRIVE Rally" },
			want:    []string{"Local Game"},
		},
		{
			name:    "unknown canonical id keeps the stored title",
			games:   []Game{{ID: "a", Title: "Local Game", CanonicalGameID: "canon-1"}},
			resolve: func(string, string) string { return "  " },
			want:    []string{"Local Game"},
		},
		{
			name: "only matching entries change",
			games: []Game{
				{ID: "a", Title: "raw name", CanonicalGameID: "canon-1"},
				{ID: "b", Title: "Kept", CanonicalGameID: "canon-2"},
			},
			resolve: func(id, _ string) string {
				if id == "canon-1" {
					return "#DRIVE Rally"
				}
				return "Kept"
			},
			want: []string{"#DRIVE Rally", "Kept"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := seedGames(t, path, tc.games)
			if err := s.SyncTitles(tc.resolve); err != nil {
				t.Fatalf("sync: %v", err)
			}
			reloaded := mustServiceAt(t, path)
			for i, want := range tc.want {
				if s.games[i].Title != want {
					t.Fatalf("memory title %d = %q, want %q", i, s.games[i].Title, want)
				}
				if reloaded.games[i].Title != want {
					t.Fatalf("stored title %d = %q, want %q", i, reloaded.games[i].Title, want)
				}
			}
		})
	}
}

func TestSyncTitlesRequiresResolver(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if err := s.SyncTitles(nil); !errors.Is(err, errNoTitleResolver) {
		t.Fatalf("err = %v, want %v", err, errNoTitleResolver)
	}
}

func TestSyncTitlesKeepsMemoryOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	s := seedGames(t, path, []Game{{ID: "a", Title: "raw name", CanonicalGameID: "canon-1"}})

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	err := s.SyncTitles(func(string, string) string { return "#DRIVE Rally" })
	if err == nil {
		t.Fatal("sync succeeded despite the write failure")
	}
	if s.games[0].Title != "raw name" {
		t.Fatalf("memory title = %q, want the value rolled back", s.games[0].Title)
	}
}
