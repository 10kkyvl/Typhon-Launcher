package catalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

func TestSameGameEditionParent(t *testing.T) {
	svc := newTestService(t)
	seed(t, svc,
		Game{ID: "base", ServerID: "srv-base", Title: "Base", ExternalIDs: ExternalIDs{IGDB: "1"}},
		Game{ID: "ed1", ServerID: "srv-ed1", ParentID: "srv-base", GameType: "Edition", Title: "Base Deluxe", ExternalIDs: ExternalIDs{IGDB: "2"}},
		Game{ID: "ed2", ServerID: "srv-ed2", ParentID: "base", GameType: "Edition", Title: "Base Ultimate", ExternalIDs: ExternalIDs{IGDB: "3"}},
		Game{ID: "orphan", ServerID: "srv-orphan", ParentID: "srv-gone", GameType: "Edition", Title: "Lone Edition", ExternalIDs: ExternalIDs{IGDB: "4"}},
		Game{ID: "twin", ServerID: "srv-twin", ParentID: "srv-gone", GameType: "Edition", Title: "Twin Edition", ExternalIDs: ExternalIDs{IGDB: "5"}},
		Game{ID: "other", ServerID: "srv-other", Title: "Other", ExternalIDs: ExternalIDs{IGDB: "6"}},
	)
	setRedirects(svc, map[string]string{"old-base": "base"})
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"edition to base", "ed1", "base", true},
		{"base to edition", "base", "ed1", true},
		{"parent by local id", "ed2", "base", true},
		{"two editions of one base", "ed1", "ed2", true},
		{"through redirect", "ed1", "old-base", true},
		{"orphan parent links nothing", "orphan", "base", false},
		{"orphan and other", "orphan", "other", false},
		{"editions of a missing parent", "orphan", "twin", false},
		{"edition and unrelated game", "ed1", "other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.SameGame(tc.a, tc.b); got != tc.want {
				t.Fatalf("SameGame(%s,%s)=%v want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
	game, err := svc.GetGame("ed1")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range game.AliasIDs {
		got[id] = true
	}
	if !got["base"] || !got["ed2"] || got["other"] || got["orphan"] {
		t.Fatalf("edition aliases=%v", game.AliasIDs)
	}
	base, err := svc.GetGame("base")
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]bool{}
	for _, id := range base.AliasIDs {
		got[id] = true
	}
	if !got["ed1"] || !got["ed2"] {
		t.Fatalf("base aliases=%v", base.AliasIDs)
	}
}

func TestEditionParentSurvivesRestartAndOldCatalogLoads(t *testing.T) {
	dir := t.TempDir()
	svc := mustServiceAt(t, dir)
	seed(t, svc,
		Game{ID: "base", ServerID: "srv-base", Title: "Base", ExternalIDs: ExternalIDs{IGDB: "1"}},
		Game{ID: "ed", ServerID: "srv-ed", ParentID: "srv-base", GameType: "Edition", Title: "Base Deluxe", ExternalIDs: ExternalIDs{IGDB: "2"}},
	)
	reopened := mustServiceAt(t, dir)
	if !reopened.SameGame("ed", "base") {
		t.Fatal("parent link lost after restart")
	}
}

func TestCatalogWithoutParentFieldLoads(t *testing.T) {
	dir := t.TempDir()
	old := []map[string]any{
		{"id": "a", "title": "Same", "sortTitle": "same", "externalIds": map[string]string{}},
		{"id": "b", "title": "Other", "sortTitle": "other", "externalIds": map[string]string{}},
	}
	if err := storage.Save(filepath.Join(dir, "catalog.json"), gamesVersion, old); err != nil {
		t.Fatal(err)
	}
	svc := mustServiceAt(t, dir)
	if len(svc.ListGames()) != 2 || svc.SameGame("a", "b") {
		t.Fatalf("old catalog misread: %+v", svc.ListGames())
	}
}

func TestPageDecodeKeepsParentID(t *testing.T) {
	var page GamePage
	if err := json.Unmarshal([]byte(`{"items":[{"id":"e","title":"E","parentId":"srv-base"}]}`), &page); err != nil {
		t.Fatal(err)
	}
	if page.Items[0].ParentID != "srv-base" {
		t.Fatalf("parentId dropped: %+v", page.Items[0])
	}
}

func TestBrowsedEditionAliasesBaseGame(t *testing.T) {
	svc := newTestService(t)
	seed(t, svc, Game{ID: "base", ServerID: "srv-base", Title: "Base", ExternalIDs: ExternalIDs{IGDB: "1"}})
	svc.SetRemoteCatalog(&remoteRegressionFixture{page: GamePage{Items: []Game{
		{ID: "srv-ed", Title: "Base Deluxe", GameType: "Edition", ParentID: "srv-base", ExternalIDs: ExternalIDs{IGDB: "2"}},
	}}})
	page, err := svc.BrowseGames(context.Background(), GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ParentID != "srv-base" || len(page.Items[0].AliasIDs) != 1 || page.Items[0].AliasIDs[0] != "base" {
		t.Fatalf("browsed edition: %+v", page.Items)
	}
}
