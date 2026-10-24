package sources

import (
	"context"
	"errors"
	"testing"

	"typhon/internal/catalog"
)

type relatedFixture struct {
	items []catalog.RelatedGame
	err   error
}

func (f *relatedFixture) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}

func (f *relatedFixture) Related(context.Context, string) ([]catalog.RelatedGame, error) {
	return f.items, f.err
}

func canonicalRelease(id, hash, canonical string) *Release {
	return &Release{ID: id, InfoHash: hash, CanonicalGameID: &canonical, RawTitle: id}
}

func TestGetRelatedReleases(t *testing.T) {
	s, cat, _ := testService(t)
	for _, g := range []catalog.Game{
		{ID: "orig", ServerID: "srv-orig", Title: "Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		{ID: "remake", ServerID: "srv-remake", Title: "Game Remake", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
		{ID: "ps", ServerID: "srv-ps", Title: "Game PS", ExternalIDs: catalog.ExternalIDs{IGDB: "3"}},
	} {
		if _, err := cat.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	cat.SetRemoteCatalog(&relatedFixture{items: []catalog.RelatedGame{
		{ID: "srv-remake", Title: "Game Remake", Relation: "remake", Platforms: []string{"PC (Microsoft Windows)"}},
		{ID: "srv-ps", Title: "Game PS", Relation: "port", Platforms: []string{"PlayStation 5"}},
		{ID: "srv-unsaved", Title: "Unsaved", Relation: "port"},
	}})
	s.mu.Lock()
	s.releases["src"] = []*Release{
		canonicalRelease("own", "h1", "orig"),
		canonicalRelease("remake-1", "h2", "remake"),
		canonicalRelease("remake-dup", "h2", "remake"),
	}
	s.mu.Unlock()

	got, err := s.GetRelatedReleases(context.Background(), "orig")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("related = %+v", got)
	}
	remake, ps, unsaved := got[0], got[1], got[2]
	if remake.GameID != "remake" || !remake.OnPC || len(remake.Groups) != 1 || len(remake.Groups[0].Duplicates) != 1 {
		t.Fatalf("remake: %+v", remake)
	}
	for _, g := range remake.Groups {
		if g.Release.ID == "own" {
			t.Fatal("own release leaked into the related block")
		}
	}
	if ps.OnPC || ps.Groups == nil || len(ps.Groups) != 0 {
		t.Fatalf("console port: %+v", ps)
	}
	if unsaved.GameID != "" || unsaved.Groups == nil || len(unsaved.Groups) != 0 || !unsaved.OnPC {
		t.Fatalf("unsaved: %+v", unsaved)
	}
	if own := s.GetReleasesForGame("orig"); len(own) != 1 || own[0].Release.ID != "own" {
		t.Fatalf("own releases changed: %+v", own)
	}
}

func TestGetRelatedReleasesSkipsSameGame(t *testing.T) {
	s, cat, _ := testService(t)
	for _, g := range []catalog.Game{
		{ID: "base", ServerID: "srv-base", Title: "Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		{ID: "edition", ServerID: "srv-ed", ParentID: "srv-base", Title: "Game Deluxe", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
	} {
		if _, err := cat.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	cat.SetRemoteCatalog(&relatedFixture{items: []catalog.RelatedGame{{ID: "srv-ed", Title: "Game Deluxe", Relation: "edition"}}})
	got, err := s.GetRelatedReleases(context.Background(), "base")
	if err != nil || len(got) != 0 {
		t.Fatalf("same-game edition listed as related: %+v %v", got, err)
	}
}

func TestGetRelatedReleasesErrors(t *testing.T) {
	boom := errors.New("boom")
	t.Run("remote error propagates", func(t *testing.T) {
		s, cat, _ := testService(t)
		if _, err := cat.AddGame(catalog.Game{ID: "g", ServerID: "srv", Title: "G"}); err != nil {
			t.Fatal(err)
		}
		cat.SetRemoteCatalog(&relatedFixture{err: boom})
		if _, err := s.GetRelatedReleases(context.Background(), "g"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		s, cat, _ := testService(t)
		if _, err := cat.AddGame(catalog.Game{ID: "g", ServerID: "srv", Title: "G"}); err != nil {
			t.Fatal(err)
		}
		cat.SetRemoteCatalog(&relatedFixture{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.GetRelatedReleases(ctx, "g"); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("empty id", func(t *testing.T) {
		s, _, _ := testService(t)
		got, err := s.GetRelatedReleases(context.Background(), "")
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %v %v", got, err)
		}
	})
}
