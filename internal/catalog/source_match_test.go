package catalog

import (
	"context"
	"errors"
	"testing"
)

type fakePartialRemote struct {
	err error
}

func (f *fakePartialRemote) Browse(context.Context, GameQuery) (GamePage, error) {
	return GamePage{}, nil
}

func (f *fakePartialRemote) MatchReleases(_ context.Context, queries []ReleaseQuery) ([]ReleaseMatch, error) {
	out := make([]ReleaseMatch, len(queries))
	var failed []int
	for i, q := range queries {
		if q.Titles[0] == "Broken Game" {
			failed = append(failed, i)
			continue
		}
		out[i].Game = &Game{ID: "server-" + q.Titles[0], Title: q.Titles[0], ExternalIDs: ExternalIDs{IGDB: q.Titles[0]}}
	}
	if len(failed) == 0 {
		return out, nil
	}
	cause := f.err
	if cause == nil {
		cause = errors.New("boom")
	}
	return out, &PartialMatchError{Failed: failed, Total: len(queries), Err: cause}
}

func TestResolveSourceQueriesPersistsSuccessesAndReportsFailedIndices(t *testing.T) {
	s := newTestService(t)
	s.SetRemoteCatalog(&fakePartialRemote{})
	queries := []ReleaseQuery{
		{Titles: []string{"Good Game"}},
		{Titles: []string{"Broken Game"}},
		{Titles: []string{"Another Good Game"}},
	}

	out, err := s.ResolveSourceQueries(context.Background(), queries)
	var partial *PartialMatchError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v, want *PartialMatchError", err)
	}
	if partial.Total != 3 || len(partial.Failed) != 1 || partial.Failed[0] != 1 {
		t.Fatalf("partial = %+v", partial)
	}
	if len(out) != 3 {
		t.Fatalf("out = %+v, want 3 entries", out)
	}
	if out[0].Status != StatusMatched || out[0].GameID != "server-Good Game" {
		t.Fatalf("query 0 = %+v, want a persisted match", out[0])
	}
	if out[2].Status != StatusMatched || out[2].GameID != "server-Another Good Game" {
		t.Fatalf("query 2 = %+v, want a persisted match", out[2])
	}
	if _, err := s.GetGame("server-Good Game"); err != nil {
		t.Fatalf("first good game not persisted: %v", err)
	}
	if _, err := s.GetGame("server-Another Good Game"); err != nil {
		t.Fatalf("second good game not persisted: %v", err)
	}
}

func TestPreviewSourceQueriesPassesThroughPartial(t *testing.T) {
	s := newTestService(t)
	s.SetRemoteCatalog(&fakePartialRemote{})
	queries := []ReleaseQuery{
		{Titles: []string{"Broken Game"}},
		{Titles: []string{"Good Game"}},
	}

	out, err := s.PreviewSourceQueries(context.Background(), queries)
	var partial *PartialMatchError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v, want *PartialMatchError", err)
	}
	if len(out) != 2 {
		t.Fatalf("out = %+v, want 2 entries", out)
	}
	if out[0].Game != nil {
		t.Fatalf("failed query 0 = %+v, want zero value", out[0])
	}
	if out[1].Game == nil || out[1].Game.ID != "server-Good Game" {
		t.Fatalf("query 1 = %+v, want the successful match", out[1])
	}
}

func TestResolveSourceQueriesFailsOutrightOnAFatalError(t *testing.T) {
	s := newTestService(t)
	s.SetRemoteCatalog(&fatalRemote{err: errors.New("boom")})

	if _, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Good Game"}}}); err == nil {
		t.Fatal("expected the fatal error to propagate")
	}
	if len(s.ListGames()) != 0 {
		t.Fatal("a fatal match error must not add games")
	}
}

type fatalRemote struct{ err error }

func (f *fatalRemote) Browse(context.Context, GameQuery) (GamePage, error) {
	return GamePage{}, nil
}

func (f *fatalRemote) MatchReleases(context.Context, []ReleaseQuery) ([]ReleaseMatch, error) {
	return nil, f.err
}
