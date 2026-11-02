package catalog

import (
	"context"
	"errors"
	"testing"
)

type consoleSecondPass struct {
	first ReleaseMatch
	err   error
	calls int
}

func (f *consoleSecondPass) Browse(context.Context, GameQuery) (GamePage, error) {
	return GamePage{}, nil
}

func (f *consoleSecondPass) MatchReleases(_ context.Context, qs []ReleaseQuery) ([]ReleaseMatch, error) {
	f.calls++
	if f.calls == 1 {
		return []ReleaseMatch{f.first}, nil
	}
	return make([]ReleaseMatch, len(qs)), f.err
}

func TestConsoleSecondPassFailure(t *testing.T) {
	accept := func(platforms []string) bool { return len(platforms) == 1 && platforms[0] == "PlayStation 2" }
	ambiguous := ReleaseMatch{Reason: ReasonAmbiguous, Candidates: []ReleaseCandidate{
		{ID: "a", Title: "Alpha", Platforms: []string{"PC"}},
		{ID: "b", Title: "Alpha", Platforms: []string{"PlayStation 2"}},
	}}
	boom := errors.New("boom")
	cases := []struct {
		name    string
		err     error
		wantErr error
		status  Status
	}{
		{"cancelled", context.Canceled, context.Canceled, ""},
		{"server failure keeps the first pass", boom, nil, StatusReview},
		{"outdated server keeps the first pass", ErrBackendOutdated, nil, StatusReview},
		{"partial answer keeps the candidate", &PartialMatchError{Failed: []int{0}, Total: 1, Err: boom}, nil, StatusReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.SetRemoteCatalog(&consoleSecondPass{first: ambiguous, err: tc.err})
			out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if out[0].Status != tc.status || len(out[0].Candidates) != 1 || out[0].Candidates[0].GameID != "b" {
				t.Fatalf("match: %+v", out[0])
			}
		})
	}
}

func TestConsoleMatchFillsMissingPlatformsOfSavedGame(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if _, err := s.AddGame(Game{ID: "local", ServerID: "srv", Title: "Alpha", ExternalIDs: ExternalIDs{IGDB: "1"}}); err != nil {
		t.Fatal(err)
	}
	platforms := []string{"PlayStation 2"}
	s.SetRemoteCatalog(&consoleSecondPass{first: ReleaseMatch{Game: &Game{ID: "srv", Title: "Alpha", Platforms: platforms, ExternalIDs: ExternalIDs{IGDB: "1"}}}})
	accept := func(got []string) bool { return len(got) == 1 && got[0] == platforms[0] }
	if _, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Alpha"}, Accept: accept}}); err != nil {
		t.Fatal(err)
	}
	game, err := s.GetGame("local")
	if err != nil {
		t.Fatal(err)
	}
	if !accept(game.Platforms) {
		t.Fatalf("platforms = %q", game.Platforms)
	}
}
