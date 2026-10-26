package typhonapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"typhon/internal/catalog"
)

func TestMatchReleasesReadsReasonAndCandidates(t *testing.T) {
	cases := []struct {
		name string
		body string
		want catalog.ReleaseMatch
	}{
		{"old server, no new fields", `{"matches":[{"game":{"id":"g1","title":"Game"}}]}`, catalog.ReleaseMatch{Game: &catalog.Game{ID: "g1", Title: "Game"}}},
		{"old server miss", `{"matches":[{}]}`, catalog.ReleaseMatch{}},
		{"none", `{"matches":[{"reason":"none"}]}`, catalog.ReleaseMatch{Reason: "none"}},
		{"ambiguous", `{"matches":[{"reason":"ambiguous","candidates":[{"id":"a","title":"A","releaseYear":2001,"platforms":["PC"],"gameType":"Main","parentId":"p"}]}]}`, catalog.ReleaseMatch{
			Reason:     "ambiguous",
			Candidates: []catalog.ReleaseCandidate{{ID: "a", Title: "A", ReleaseYear: 2001, Platforms: []string{"PC"}, GameType: "Main", ParentID: "p"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := New(server.URL, func() (string, error) { return "", nil })
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.MatchReleases(context.Background(), []catalog.ReleaseQuery{{Titles: []string{"x"}}})
			if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], tc.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
