package catalog

import (
	"context"
	"testing"
)

type matchFixture struct {
	games map[string]Game
}

func (f *matchFixture) Browse(context.Context, GameQuery) (GamePage, error) { return GamePage{}, nil }

func (f *matchFixture) MatchReleases(_ context.Context, queries []ReleaseQuery) ([]ReleaseMatch, error) {
	out := make([]ReleaseMatch, len(queries))
	for i, q := range queries {
		if g, ok := f.games[q.Titles[0]]; ok {
			game := g
			out[i].Game = &game
		}
	}
	return out, nil
}

func seedLocalEdition(t *testing.T, dir string) *Service {
	t.Helper()
	s := mustServiceAt(t, dir)
	seed(t, s,
		Game{ID: "base", ServerID: "srv-base", Title: "Base Game", ExternalIDs: ExternalIDs{IGDB: "10"}},
		Game{ID: "edition", ServerID: "srv-edition", Title: "Base Game Deluxe", ExternalIDs: ExternalIDs{IGDB: "11"}},
	)
	s.SetRemoteCatalog(&matchFixture{games: map[string]Game{
		"Base Game Deluxe": {ID: "srv-edition", Title: "Base Game Deluxe", GameType: "Edition", ParentID: "srv-base", ExternalIDs: ExternalIDs{IGDB: "11"}},
	}})
	return s
}

func TestResolveSourceQueriesAdoptsParentOfSavedEdition(t *testing.T) {
	dir := t.TempDir()
	s := seedLocalEdition(t, dir)
	if s.SameGame("base", "edition") {
		t.Fatal("precondition: edition without ParentID must not be the same game as its base")
	}

	out, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Base Game Deluxe"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].GameID != "edition" {
		t.Fatalf("matched %q, want the saved record %q", out[0].GameID, "edition")
	}
	if !s.SameGame("base", "edition") {
		t.Fatal("edition is not the same game as its base after the match answer carried parentId")
	}
	if list := s.idx.byParent["srv-base"]; len(list) != 1 {
		t.Fatalf("byParent[srv-base] = %v, want one posting", list)
	}

	reopened := mustServiceAt(t, dir)
	g, ok := reopened.idx.game("edition")
	if !ok || g.ParentID != "srv-base" || g.GameType != "Edition" {
		t.Fatalf("after restart edition = %+v, want ParentID srv-base and GameType Edition", g)
	}
}

func TestResolveSourceQueriesKeepsSavedMetadataOfAMatchedGame(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	seed(t, s, Game{
		ID: "edition", ServerID: "srv-edition", Title: "Base Game Deluxe", ExternalIDs: ExternalIDs{IGDB: "11"},
		CoverURL: "https://cdn.test/cover.jpg", Summary: "A summary", Developer: "Studio", Genres: []string{"Adventure"},
	})
	s.SetRemoteCatalog(&matchFixture{games: map[string]Game{
		"Base Game Deluxe": {ID: "srv-edition", Title: "Base Game Deluxe", GameType: "Edition", ParentID: "srv-base", ExternalIDs: ExternalIDs{IGDB: "11"}},
	}})

	if _, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Base Game Deluxe"}}}); err != nil {
		t.Fatal(err)
	}
	g, ok := s.idx.game("edition")
	if !ok || g.ParentID != "srv-base" {
		t.Fatalf("edition = %+v, want ParentID srv-base", g)
	}
	if g.CoverURL != "https://cdn.test/cover.jpg" || g.Summary != "A summary" || g.Developer != "Studio" || len(g.Genres) != 1 {
		t.Fatalf("a slim match answer wiped saved metadata: %+v", g)
	}
}

func TestResolveSourceQueriesRollsBackParentOnPersistFailure(t *testing.T) {
	s := seedLocalEdition(t, t.TempDir())
	s.gamesPath = t.TempDir() // atomic rename over a directory must fail

	if _, err := s.ResolveSourceQueries(context.Background(), []ReleaseQuery{{Titles: []string{"Base Game Deluxe"}}}); err == nil {
		t.Fatal("persist failure was not returned")
	}
	g, ok := s.idx.game("edition")
	if !ok || g.ParentID != "" || g.GameType != "" {
		t.Fatalf("edition after failed persist = %+v, want the original record", g)
	}
	if len(s.idx.byParent["srv-base"]) != 0 {
		t.Fatal("index kept a posting for the rolled back parent")
	}
	if s.SameGame("base", "edition") {
		t.Fatal("rolled back edition still counts as the same game as its base")
	}
}
