package catalog

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestServerCandidatesDropsAndCutsBadInput(t *testing.T) {
	long := strings.Repeat("я", 500)
	one := Candidate{GameID: "1", Title: "A", Method: MethodNone, Source: SourceServer}
	cases := []struct {
		name string
		in   []ReleaseCandidate
		want []Candidate
	}{
		{"empty", nil, []Candidate{}},
		{"empty id dropped", []ReleaseCandidate{{ID: " ", Title: "A"}}, []Candidate{}},
		{"empty title dropped", []ReleaseCandidate{{ID: "1", Title: "\x00\n"}}, []Candidate{}},
		{"too long id dropped", []ReleaseCandidate{{ID: strings.Repeat("9", maxCandidateID+1), Title: "A"}}, []Candidate{}},
		{"duplicate id dropped", []ReleaseCandidate{{ID: "1", Title: "A"}, {ID: "1", Title: "B"}}, []Candidate{one}},
		{"at most three", []ReleaseCandidate{{ID: "1", Title: "A"}, {ID: "2", Title: "B"}, {ID: "3", Title: "C"}, {ID: "4", Title: "D"}, {ID: "5", Title: "E"}}, []Candidate{
			one,
			{GameID: "2", Title: "B", Method: MethodNone, Source: SourceServer},
			{GameID: "3", Title: "C", Method: MethodNone, Source: SourceServer},
		}},
		{"long title cut", []ReleaseCandidate{{ID: "1", Title: long}}, []Candidate{{GameID: "1", Title: strings.Repeat("я", maxCandidateTitle), Method: MethodNone, Source: SourceServer}}},
		{"control chars stripped", []ReleaseCandidate{{ID: "1", Title: "A\x1b\tB"}}, []Candidate{{GameID: "1", Title: "AB", Method: MethodNone, Source: SourceServer}}},
		{"bidi override stripped", []ReleaseCandidate{{ID: "1", Title: "A\u202eB\u200b"}}, []Candidate{{GameID: "1", Title: "AB", Method: MethodNone, Source: SourceServer}}},
		{"absurd year dropped", []ReleaseCandidate{{ID: "1", Title: "A", ReleaseYear: 99999}}, []Candidate{one}},
		{"platforms cleaned", []ReleaseCandidate{{ID: "1", Title: "A", Platforms: []string{"PC", "", strings.Repeat("x", 80)}}}, []Candidate{{GameID: "1", Title: "A", Method: MethodNone, Source: SourceServer, Platforms: []string{"PC", strings.Repeat("x", maxCandidatePlat)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serverCandidates(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNormalizeReasonDropsUnknownWords(t *testing.T) {
	for in, want := range map[string]string{"": "", "none": "none", "ambiguous": "ambiguous", "too_many": "too_many", "weird": ""} {
		if got := normalizeReason(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

type answerFixture struct{ answers []ReleaseMatch }

func (f *answerFixture) Browse(context.Context, GameQuery) (GamePage, error) { return GamePage{}, nil }

func (f *answerFixture) MatchReleases(context.Context, []ReleaseQuery) ([]ReleaseMatch, error) {
	return f.answers, nil
}

func TestResolveSourceQueriesReadsReasonAndCandidates(t *testing.T) {
	good := []ReleaseCandidate{{ID: "a", Title: "Alpha", ReleaseYear: 2001}, {ID: "b", Title: "Beta"}}
	s := mustServiceAt(t, t.TempDir())
	s.SetRemoteCatalog(&answerFixture{answers: []ReleaseMatch{
		{},
		{Reason: ReasonNone},
		{Reason: ReasonTooMany, Candidates: good},
		{Reason: ReasonAmbiguous, Candidates: good},
		{Reason: ReasonAmbiguous, Candidates: []ReleaseCandidate{{ID: "", Title: "Broken"}}},
		{Reason: "from-the-future", Candidates: good},
	}})
	queries := make([]ReleaseQuery, 6)
	for i := range queries {
		queries[i] = ReleaseQuery{Titles: []string{"t"}}
	}
	out, err := s.ResolveSourceQueries(context.Background(), queries)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		status     Status
		reason     string
		candidates int
	}
	want := []outcome{
		{StatusUnmatched, "", 0},
		{StatusUnmatched, ReasonNone, 0},
		{StatusUnmatched, ReasonTooMany, 0},
		{StatusReview, ReasonAmbiguous, 2},
		{StatusUnmatched, ReasonAmbiguous, 0},
		{StatusUnmatched, "", 0},
	}
	for i, m := range out {
		got := outcome{m.Status, m.Reason, len(m.Candidates)}
		if got != want[i] || m.GameID != "" {
			t.Fatalf("answer %d: %+v (%+v), want %+v", i, got, m, want[i])
		}
	}
	if out[3].Candidates[0].GameID != "a" || *out[3].Candidates[0].Year != 2001 {
		t.Fatalf("candidate: %+v", out[3].Candidates[0])
	}
	if s.HasGame("a") || s.HasGame("b") {
		t.Fatal("candidates must not enter the local catalog")
	}
}

func TestResolveSourceQueriesTranslatesCandidateServerIDs(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	seed(t, s, Game{ID: "local-1", ServerID: "srv-1", Title: "Known Game", ExternalIDs: ExternalIDs{IGDB: "5"}})
	s.SetRemoteCatalog(&answerFixture{answers: []ReleaseMatch{{
		Reason:     ReasonAmbiguous,
		Candidates: []ReleaseCandidate{{ID: "srv-1", Title: "Known Game"}, {ID: "srv-unknown", Title: "Unknown Game"}},
	}}})
	out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"t"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := out[0].Candidates
	if len(got) != 2 || got[0].GameID != "local-1" || got[1].GameID != "srv-unknown" {
		t.Fatalf("candidates: %+v", got)
	}
	if !s.HasGame(got[0].GameID) || s.HasGame(got[1].GameID) {
		t.Fatal("known candidate must resolve in the catalog, unknown must not")
	}
}
