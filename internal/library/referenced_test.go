package library

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestReferencedCatalogIDs(t *testing.T) {
	cases := []struct {
		name     string
		games    []string
		archived []string
		want     []string
	}{
		{name: "empty", want: []string{}},
		{name: "empty ids are skipped", games: []string{"", "g1"}, archived: []string{""}, want: []string{"g1"}},
		{
			name:     "live and archived merge, duplicates collapse, sorted",
			games:    []string{"zz", "mm", "zz"},
			archived: []string{"aa", "mm"},
			want:     []string{"aa", "mm", "zz"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			s.mu.Lock()
			for _, id := range tc.games {
				s.games = append(s.games, Game{CanonicalGameID: id})
			}
			for _, id := range tc.archived {
				s.archived = append(s.archived, Game{CanonicalGameID: id})
			}
			s.mu.Unlock()
			got := s.ReferencedCatalogIDs()
			if got == nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ReferencedCatalogIDs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
