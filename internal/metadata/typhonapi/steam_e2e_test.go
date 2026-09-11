package typhonapi

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"typhon/internal/catalog"
	"typhon/internal/metadata"
)

// Optional read-only backend smoke check. All launcher files use t.TempDir;
// no account session, user library, or running Wails app is accessed.
func TestSteamBackendToLauncher(t *testing.T) {
	base := os.Getenv("TYPHON_STEAM_E2E_URL")
	if base == "" {
		t.Skip("TYPHON_STEAM_E2E_URL not set")
	}
	client, e := New(base, func() (string, error) { return "", nil })
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	cat, e := catalog.NewServiceAt(dir)
	if e != nil {
		t.Fatal(e)
	}
	g, e := cat.AddGame(catalog.Game{Title: "Portal 2"})
	if e != nil {
		t.Fatal(e)
	}
	svc, e := metadata.NewServiceAt(dir, cat, client)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.ServiceStartup(context.Background(), application.ServiceOptions{}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := svc.ServiceShutdown(); e != nil {
			t.Error(e)
		}
	})
	svc.SetLanguage("ru")
	view, e := svc.ApplyMatch(g.ID, "steam:620")
	if e != nil {
		t.Fatal(e)
	}
	if view.Game.ExternalIDs.Steam != "620" || view.Game.ExternalIDs.IGDB != "" || view.Game.Summary == "" || view.Cover == "" || view.Hero == "" || len(view.Screenshots) == 0 {
		t.Fatalf("incomplete card: steam=%q igdb=%q cover=%q hero=%q shots=%d", view.Game.ExternalIDs.Steam, view.Game.ExternalIDs.IGDB, view.Cover, view.Hero, len(view.Screenshots))
	}
	if view.Game.MetadataLanguage != "ru" || !strings.Contains(view.Game.Summary, "тестирования") {
		t.Fatalf("Russian card missing: %q", view.Game.Summary)
	}
	ruSummary := view.Game.Summary
	svc.SetLanguage("en")
	english, e := svc.Refresh(g.ID)
	if e != nil || english.Game.MetadataLanguage != "en" || english.Game.Summary == ruSummary {
		t.Fatalf("language switch failed: %+v %v", english.Game, e)
	}
	if view.Game.MetadataPartial {
		t.Fatal("required artwork is incomplete")
	}
	reopened, e := catalog.NewServiceAt(dir)
	if e != nil {
		t.Fatal(e)
	}
	persisted, e := reopened.GetGame(g.ID)
	if e != nil {
		t.Fatal(e)
	}
	if persisted.ExternalIDs.Steam != "620" || persisted.CoverAssetID == "" {
		t.Fatal("card was not persisted")
	}
	candidates, e := client.Search(context.Background(), "Portal 2", 10)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range candidates {
		if c.ProviderID == "steam:620" {
			found = true
		}
	}
	if !found {
		t.Fatal("stored Steam card missing from search")
	}
	resolved, e := client.Resolve(context.Background(), []string{"Portal 2"})
	if e != nil || len(resolved) != 1 {
		t.Fatalf("bulk resolve: %v %v", resolved, e)
	}
	t.Logf("HTTP backend → real Steam → PostgreSQL → launcher: persisted Steam ID, cover, hero and %d screenshots; search and bulk resolve passed", len(view.Screenshots))
}
