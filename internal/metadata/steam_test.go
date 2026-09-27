package metadata

import (
	"testing"
	"typhon/internal/catalog"
)

func TestSteamMetadataPersistsAndRefreshesBySteamID(t *testing.T) {
	provider := &fakeProvider{meta: map[string]GameMetadata{"steam:620": {ProviderID: "steam:620", SteamAppID: "620", Title: "Portal 2", Summary: "Steam description"}}}
	svc, cat, dir := newTestService(t, provider)
	g := addGame(t, cat, catalog.Game{Title: "Portal 2"})
	view, e := svc.ApplyMatch(g.ID, "steam:620")
	if e != nil {
		t.Fatal(e)
	}
	if view.Game.ExternalIDs.IGDB != "" || view.Game.ExternalIDs.Steam != "620" {
		t.Fatalf("wrong identity %+v", view.Game.ExternalIDs)
	}
	reloaded, e := catalog.NewServiceAt(dir)
	if e != nil {
		t.Fatal(e)
	}
	stored, e := reloaded.GetGame(g.ID)
	if e != nil {
		t.Fatal(e)
	}
	if stored.ExternalIDs.Steam != "620" || stored.Summary != "Steam description" {
		t.Fatalf("not persisted %+v", stored)
	}
}
