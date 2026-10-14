package sources

import (
	"slices"
	"testing"
)

func TestReferencedGameIDs(t *testing.T) {
	str := func(v string) *string { return &v }
	cases := []struct {
		name     string
		releases []*Release
		want     []string
	}{
		{name: "no releases", want: []string{}},
		{
			name: "canonical ids and remote matches are distinct and sorted",
			releases: []*Release{
				{ID: "a", CanonicalGameID: str("g2")},
				{ID: "b", CanonicalGameID: str("g1"), RemoteMatch: &RemoteMatch{GameID: "g3"}},
				{ID: "c", CanonicalGameID: str("g2"), RemoteMatch: &RemoteMatch{GameID: "g1"}},
			},
			want: []string{"g1", "g2", "g3"},
		},
		{
			name: "unmatched releases contribute nothing",
			releases: []*Release{
				{ID: "a"},
				{ID: "b", CanonicalGameID: str("")},
				{ID: "c", RemoteMatch: &RemoteMatch{}},
			},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := testService(t)
			s.mu.Lock()
			s.releases["src"] = tc.releases
			s.mu.Unlock()
			if got := s.ReferencedGameIDs(); !slices.Equal(got, tc.want) {
				t.Fatalf("ReferencedGameIDs = %v, want %v", got, tc.want)
			}
		})
	}
}
