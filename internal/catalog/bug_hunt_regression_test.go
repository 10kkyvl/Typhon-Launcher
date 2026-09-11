package catalog

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyMetadataKeepsIGDBWhenSteamOnly(t *testing.T) {
	service, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	game, err := service.AddGame(Game{
		ID:          "local",
		Title:       "Known game",
		ExternalIDs: ExternalIDs{IGDB: "igdb-42", Steam: "steam-old"},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.ApplyMetadata(game.ID, MetadataPatch{
		SteamID:   "steam-new",
		Title:     "Known game",
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ExternalIDs.IGDB != "igdb-42" || updated.ExternalIDs.Steam != "steam-new" {
		t.Fatalf("provider ids = %+v, want existing IGDB and new Steam", updated.ExternalIDs)
	}
	reloaded, err := NewServiceAt(filepath.Dir(service.gamesPath))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.GetGame(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExternalIDs.IGDB != "igdb-42" {
		t.Fatalf("persisted IGDB = %q, want it preserved", stored.ExternalIDs.IGDB)
	}
}

type remoteRegressionFixture struct {
	page GamePage
	err  error
}

func (f *remoteRegressionFixture) Browse(context.Context, GameQuery) (GamePage, error) {
	return f.page, f.err
}

func TestBrowseKeepsLearnedAliasAndLocalSteamOnIncompleteProvider(t *testing.T) {
	service, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local, err := service.AddGame(Game{
		ID:          "local",
		Title:       "Official title",
		Aliases:     []string{"learned release"},
		ExternalIDs: ExternalIDs{IGDB: "42", Steam: "730"},
	})
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteRegressionFixture{page: GamePage{
		Items:     []Game{{ID: "server-42", Title: "Official title", ExternalIDs: ExternalIDs{IGDB: "42"}}},
		Providers: []IndexStatus{{Provider: "igdb", Complete: true}, {Provider: "steam", Complete: false}},
	}}
	service.SetRemoteCatalog(remote)
	page, err := service.BrowseGames(GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != local.ID {
		t.Fatalf("page item = %+v, want local identity", page.Items)
	}
	got, err := service.GetGame(local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalIDs.Steam != "730" {
		t.Fatalf("Steam id = %q, want local id", got.ExternalIDs.Steam)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "learned release" {
		t.Fatalf("aliases = %v, want learned alias", got.Aliases)
	}
}

func TestBrowseDoesNotPersistUnchangedRemotePage(t *testing.T) {
	service, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteRegressionFixture{page: GamePage{Items: []Game{{ID: "server", Title: "Stable", ExternalIDs: ExternalIDs{IGDB: "1"}}}}}
	service.SetRemoteCatalog(remote)
	if _, err := service.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectsAreCanonicalInPublicQueries(t *testing.T) {
	service, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddGame(Game{ID: "old", Title: "Merged title"}); err != nil {
		t.Fatal(err)
	}
	canonical, err := service.AddGame(Game{ID: "new", Title: "Merged title"})
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.redirects = map[string]string{}
	service.redirects["old"] = canonical.ID
	service.rebuildLocked()
	service.mu.Unlock()
	page := service.QueryGames(GameQuery{PageSize: 20})
	if len(page.Items) != 1 || page.Items[0].ID != canonical.ID {
		t.Fatalf("query items = %+v, want canonical only", page.Items)
	}
	got := service.GetGames([]string{"old", canonical.ID})
	if len(got) != 1 || got[0].ID != canonical.ID {
		t.Fatalf("GetGames = %+v, want one canonical item", got)
	}
}
