package sources

import (
	"context"
	"testing"
	"typhon/internal/catalog"
)

func TestProviderCatalogMembershipDoesNotFollowSources(t *testing.T) {
	s, cat, _ := testService(t)
	for _, g := range []catalog.Game{{ID: "112", Title: "112 Operator", ExternalIDs: catalog.ExternalIDs{IGDB: "112"}}, {ID: "11f", Title: "11F", ExternalIDs: catalog.ExternalIDs{Steam: "1847120"}}, {ID: "no-downloads", Title: "Official game without downloads", ExternalIDs: catalog.ExternalIDs{IGDB: "999"}}} {
		if _, err := cat.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	server := newFeedServer(t, feedBody(t, "Catalog membership fixture",
		feedEntry{Title: "112 Operator", URIs: []string{magnetOf("a1")}},
		feedEntry{Title: "112 Operator [Папка игры] PC | Лицензия", URIs: []string{magnetOf("a2")}},
		feedEntry{Title: "112 Operator v 0.250801 [Архив]", URIs: []string{magnetOf("a3")}},
		feedEntry{Title: "11F", URIs: []string{magnetOf("b1")}},
		feedEntry{Title: "11F (+ Windows 7 Fix, )", URIs: []string{magnetOf("b2")}},
		feedEntry{Title: "Unknown release v1.0", URIs: []string{magnetOf("c1")}},
	))
	src := addSource(t, s, server.url())
	if len(cat.ListGames()) != 3 {
		t.Fatal("import changed provider catalog membership")
	}
	for id, want := range map[string]int{"112": 3, "11f": 2, "no-downloads": 0} {
		if got := len(s.GetReleasesForGame(id)); got != want {
			t.Fatalf("%s release groups=%d want %d", id, got, want)
		}
	}
	unknown := releasesOf(t, s, src.ID, "unmatched")
	if len(unknown) != 1 || unknown[0].Release.RawTitle != "Unknown release v1.0" {
		t.Fatalf("unknown source records: %+v", unknown)
	}
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
	if len(cat.ListGames()) != 3 || len(s.GetReleasesForGame("112")) != 3 {
		t.Fatal("repeat import changed identities")
	}
	if err := s.RemoveSource(src.ID); err != nil {
		t.Fatal(err)
	}
	if len(cat.ListGames()) != 3 {
		t.Fatal("removing source removed a provider game")
	}
}

type membershipRemote struct{}

func (membershipRemote) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}
func TestRemoteManualConfirmationDoesNotBindOtherHomonymousReleases(t *testing.T) {
	s, cat, _ := testService(t)
	if _, err := cat.AddGame(catalog.Game{ID: "one", Title: "Same Name", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}}); err != nil {
		t.Fatal(err)
	}
	cat.SetRemoteCatalog(membershipRemote{})
	server := newFeedServer(t, feedBody(t, "Homonyms", feedEntry{Title: "Same Name v1.0", URIs: []string{magnetOf("aa")}}, feedEntry{Title: "Same Name v2.0", URIs: []string{magnetOf("bb")}}))
	src := addSource(t, s, server.url())
	page := s.QueryReleases(ReleaseQuery{SourceID: src.ID})
	if len(page.Items) != 2 {
		t.Fatal("fixture releases missing")
	}
	if err := s.ConfirmMatch(page.Items[0].Release.ID, "one"); err != nil {
		t.Fatal(err)
	}
	groups := s.GetReleasesForGame("one")
	if len(groups) != 1 || groups[0].Release.ID != page.Items[0].Release.ID {
		t.Fatal("manual choice attached unconfirmed homonym")
	}
}
