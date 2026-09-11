package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"typhon/internal/uierr"
)

type remoteFixture struct {
	page  GamePage
	err   error
	calls int
}

func (f *remoteFixture) Browse(context.Context, GameQuery) (GamePage, error) {
	f.calls++
	return f.page, f.err
}
func TestRemotePagesDoNotExposePrivateGamesAndKeepOfflineCache(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AddGame(Game{ID: "private", Title: "Unknown local install"}); err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{{ID: "server", Title: "Official (Edition)", ExternalIDs: ExternalIDs{IGDB: "12"}}}, Total: 1, Page: 1, PageSize: 60, Revision: 2}}
	svc.SetRemoteCatalog(remote)
	q := GameQuery{Page: 1, PageSize: 60}
	page, err := svc.BrowseGames(q)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "server" {
		t.Fatalf("remote page %+v %v", page, err)
	}
	remote.err = errors.New("network down")
	page, err = svc.BrowseGames(q)
	if err != nil || !page.Offline || page.Items[0].Title != "Official (Edition)" {
		t.Fatalf("offline cache %+v %v", page, err)
	}
	if _, err = svc.BrowseGames(GameQuery{Search: "uncached"}); err == nil {
		t.Fatal("uncached network failure reported as empty success")
	}
	remote.err = ErrCatalogChanged
	if _, err = svc.BrowseGames(q); !errors.Is(err, ErrCatalogChanged) {
		t.Fatal("revision conflict hidden by cache")
	}
	if uierr.Code(err) != "catalog.changed" {
		t.Fatalf("missing frontend recovery code: %v", err)
	}
	if _, err = svc.GetGame("private"); err != nil {
		t.Fatal("private game lost")
	}
}
func TestLateProviderLinkAndCorrectionKeepLocalReferences(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old, err := svc.AddGame(Game{ID: "steam-home", Title: "11F", ExternalIDs: ExternalIDs{Steam: "11"}})
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{{ID: "igdb-home", Title: "11F", ExternalIDs: ExternalIDs{IGDB: "22", Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11", "12"}}}}, Total: 1}}
	svc.SetRemoteCatalog(remote)
	page, err := svc.BrowseGames(GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].ID == old.ID {
		t.Fatal("Steam home ID reassigned to another provider")
	}
	if !svc.SameGame(old.ID, "igdb-home") {
		t.Fatal("confirmed link not resolved")
	}
	detail, err := svc.GetGame("igdb-home")
	if err != nil || len(detail.AliasIDs) != 1 || detail.AliasIDs[0] != old.ID {
		t.Fatalf("detail aliases: %+v %v", detail, err)
	}
	remote.page.Items[0].ProviderLinks = nil
	remote.page.Items[0].ExternalIDs.Steam = ""
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if svc.SameGame(old.ID, "igdb-home") {
		t.Fatal("corrected link still merges games")
	}
}

func TestPartialRemoteCacheDoesNotConfirmTitleIdentity(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "one", Title: "Same Name", ExternalIDs: ExternalIDs{IGDB: "11"}}}}})
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if got := svc.Resolve(Query{Title: "Same Name"}); got.Status != StatusReview || got.GameID != "" {
		t.Fatalf("partial cache auto matched: %+v", got)
	}
	if err = svc.LearnMatch("same name", "one"); err != nil {
		t.Fatal(err)
	}
	if got := svc.Resolve(Query{Title: "Same Name"}); got.Status != StatusMatched {
		t.Fatalf("manual confirmation lost: %+v", got)
	}
}

func TestCorrectedProviderClaimInvalidatesAnUnvisitedOldCard(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []Game{{ID: "steam", Title: "Store", ExternalIDs: ExternalIDs{Steam: "11"}}, {ID: "wrong", Title: "Wrong homonym", ExternalIDs: ExternalIDs{IGDB: "1", Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11"}}}} {
		if _, err = svc.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	svc.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "correct", Title: "Correct game", ExternalIDs: ExternalIDs{IGDB: "2", Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11"}}}}}})
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if svc.SameGame("steam", "wrong") || !svc.SameGame("steam", "correct") {
		t.Fatal("old page retained a disproven provider link")
	}
	reopened, err := NewServiceAt(filepath.Dir(svc.gamesPath))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.SameGame("steam", "wrong") {
		t.Fatal("disproven link survived restart")
	}
}

func TestProviderMergeUndoInvalidatesCachedIGDBAlias(t *testing.T) {
	svc, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []Game{{ID: "old", Title: "Old", ExternalIDs: ExternalIDs{IGDB: "1"}}, {ID: "target", ServerID: "target", Title: "Target", ExternalIDs: ExternalIDs{IGDB: "2"}, ProviderLinks: map[string][]string{"igdb": {"1", "2"}}}} {
		if _, err = svc.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	if !svc.SameGame("old", "target") {
		t.Fatal("fixture merge missing")
	}
	svc.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "old", Title: "Old", ExternalIDs: ExternalIDs{IGDB: "1"}, ProviderLinks: map[string][]string{"igdb": {"1"}}}}}})
	if _, err = svc.BrowseGames(GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if svc.SameGame("old", "target") {
		t.Fatal("undone IGDB merge remained in cached target")
	}
}
