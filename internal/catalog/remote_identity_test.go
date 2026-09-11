package catalog

import (
	"testing"
	"time"
)

// The index being incomplete does not weaken an explicit provider link.
func TestIncompleteIndexAcceptsExplicitNewLink(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteRegressionFixture{page: GamePage{
		Items:     []Game{{ID: "igdb-card", Title: "0xFF", ExternalIDs: ExternalIDs{IGDB: "242303"}, ProviderLinks: map[string][]string{"igdb": {"242303"}}}},
		Providers: []IndexStatus{{Provider: "igdb", Complete: false}, {Provider: "steam", Complete: false}},
	}}
	svc.SetRemoteCatalog(remote)
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	remote.page.Items = []Game{{ID: "igdb-card", Title: "0xFF", ExternalIDs: ExternalIDs{IGDB: "242303", Steam: "2218760"}, ProviderLinks: map[string][]string{"igdb": {"242303"}, "steam": {"2218760"}}}}
	page, err := svc.BrowseGames(GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].ExternalIDs.Steam != "2218760" {
		t.Errorf("Browse dropped explicit new Steam link: got %+v links=%v", page.Items[0].ExternalIDs, page.Items[0].ProviderLinks)
	}
	restarted, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	game, err := restarted.GetGame("igdb-card")
	if err != nil {
		t.Fatal(err)
	}
	if game.ExternalIDs.Steam != "2218760" {
		t.Errorf("Persisted catalog dropped explicit new Steam link: got %+v links=%v", game.ExternalIDs, game.ProviderLinks)
	}
}

func TestIncompleteIndexCorrectionRemovesStaleClaim(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []Game{
		{ID: "steam", ServerID: "steam", Title: "Same", ExternalIDs: ExternalIDs{Steam: "11"}},
		{ID: "wrong", ServerID: "wrong", Title: "Same", ExternalIDs: ExternalIDs{IGDB: "1", Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11"}}},
	} {
		if _, err = svc.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	svc.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "right", Title: "Same", ExternalIDs: ExternalIDs{IGDB: "2", Steam: "11"}, ProviderLinks: map[string][]string{"igdb": {"2"}, "steam": {"11"}}}}, Providers: []IndexStatus{{Provider: "igdb", Complete: false}, {Provider: "steam", Complete: false}}}})
	page, err := svc.BrowseGames(GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.SameGame("wrong", "steam") || !svc.SameGame("right", "steam") {
		t.Fatal("explicit correction did not replace conflicting claim")
	}
	if len(page.Items[0].AliasIDs) != 1 || page.Items[0].AliasIDs[0] != "steam" {
		t.Fatalf("personal aliases=%v", page.Items[0].AliasIDs)
	}
}

func TestOfflinePageFoldsLearnedProviderPairAndPreservesHomonym(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{
		{ID: "steam", Title: "Same", ExternalIDs: ExternalIDs{Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11"}}},
		{ID: "igdb", Title: "Same", ExternalIDs: ExternalIDs{IGDB: "2"}, ProviderLinks: map[string][]string{"igdb": {"2"}}},
		{ID: "homonym", Title: "Same", ExternalIDs: ExternalIDs{IGDB: "3"}},
	}, Total: 3, Page: 1, PageSize: 60}}
	svc.SetRemoteCatalog(remote)
	query := GameQuery{Page: 1, PageSize: 60}
	if _, err = svc.BrowseGames(query); err != nil {
		t.Fatal(err)
	}
	remote.page.Items = []Game{{ID: "igdb", Title: "Same", Developer: "Author", ExternalIDs: ExternalIDs{IGDB: "2", Steam: "11"}, ProviderLinks: map[string][]string{"igdb": {"2"}, "steam": {"11"}}}}
	if _, err = svc.BrowseGames(GameQuery{Search: "Same"}); err != nil {
		t.Fatal(err)
	}
	// Reopen without a remote: use the original three-row page and durable links.
	svc, err = NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.BrowseGames(query)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Offline || len(page.Items) != 2 || page.Total != 2 {
		t.Fatalf("offline page=%+v", page)
	}
	if page.Items[0].ID != "igdb" || page.Items[0].Developer != "Author" || len(page.Items[0].AliasIDs) != 1 || page.Items[0].AliasIDs[0] != "steam" {
		t.Fatalf("canonical offline row=%+v", page.Items[0])
	}
	if page.Items[1].ID != "homonym" {
		t.Fatal("same title incorrectly merged")
	}
	if _, err = svc.GetGame("steam"); err != nil {
		t.Fatal("personal reference deleted", err)
	}
}

func TestBrowseDeveloperOverridesOldEmptyDetails(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{{ID: "game", Title: "Game", ExternalIDs: ExternalIDs{IGDB: "2"}}}}}
	svc.SetRemoteCatalog(remote)
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApplyMetadata("game", MetadataPatch{IGDBID: "2", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	remote.page.Items[0].Developer = "Author"
	page, err := svc.BrowseGames(GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].Developer != "Author" {
		t.Fatalf("developer=%q", page.Items[0].Developer)
	}
}
