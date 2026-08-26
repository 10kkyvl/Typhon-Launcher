package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizeDropsJunkAliases(t *testing.T) {
	games := []Game{
		{ID: "a", Title: "Dark Souls III", Aliases: []string{"9 souls", "dark souls 3"}},
		{ID: "b", Title: "Prey", Aliases: []string{"prey 2017"}},
	}
	got, changed := sanitize(games)
	if !changed {
		t.Fatal("junk alias was kept")
	}
	if want := []string{"dark souls 3"}; len(got[0].Aliases) != 1 || got[0].Aliases[0] != want[0] {
		t.Fatalf("aliases = %v, want %v", got[0].Aliases, want)
	}
	if len(got[1].Aliases) != 1 {
		t.Fatalf("aliases = %v, want the close alias kept", got[1].Aliases)
	}
}

func TestSanitizeRevertsPoisonedDuplicate(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	year := 2019
	games := []Game{
		{ID: "a", Title: "Days Gone", ExternalIDs: ExternalIDs{IGDB: "19561"}, CreatedAt: older.Add(time.Hour)},
		{
			ID:           "b",
			Title:        "Days Gone",
			SortTitle:    "days gone",
			Aliases:      []string{"63 days"},
			ExternalIDs:  ExternalIDs{IGDB: "19561"},
			Summary:      "wrong",
			Developer:    "Bend Studio",
			Genres:       []string{"Action"},
			ReleaseYear:  &year,
			CoverAssetID: "cover",
			CreatedAt:    older,
		},
	}
	got, changed := sanitize(games)
	if !changed {
		t.Fatal("duplicate was left as is")
	}
	if got[0].Title != "Days Gone" || got[0].ExternalIDs.IGDB != "19561" {
		t.Fatalf("survivor = %+v", got[0])
	}
	poisoned := got[1]
	if poisoned.Title != "63 days" || poisoned.SortTitle != "63 days" {
		t.Fatalf("title = %q, want the original feed title", poisoned.Title)
	}
	if !poisoned.Provisional {
		t.Fatal("reverted game is not provisional")
	}
	if poisoned.ExternalIDs.IGDB != "" || poisoned.Summary != "" || poisoned.Developer != "" ||
		poisoned.Genres != nil || poisoned.ReleaseYear != nil || poisoned.CoverAssetID != "" ||
		poisoned.Aliases != nil {
		t.Fatalf("foreign metadata survived the revert: %+v", poisoned)
	}
}

func TestSanitizeKeepsCleanDuplicate(t *testing.T) {
	games := []Game{
		{ID: "a", Title: "Airborne Kingdom", ExternalIDs: ExternalIDs{IGDB: "115473"}},
		{ID: "b", Title: "Airborne Kingdom", ExternalIDs: ExternalIDs{IGDB: "115473"}, Aliases: []string{"airbone kingdom"}},
	}
	got, changed := sanitize(games)
	if changed {
		t.Fatalf("clean duplicate was rewritten: %+v", got)
	}
	if got[1].Provisional || got[1].ExternalIDs.IGDB != "115473" {
		t.Fatalf("game = %+v", got[1])
	}
}

func TestServiceSanitizesOnLoad(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	game, err := svc.AddGame(Game{Title: "The Incredible Adventures of Van Helsing", Aliases: []string{"adorable adventures"}})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	reloaded, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	if match := reloaded.Resolve(Query{Title: "Adorable Adventures"}); match.Status == StatusMatched {
		t.Fatalf("junk alias still matches: %+v", match)
	}
	stored, err := reloaded.GetGame(game.ID)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if len(stored.Aliases) != 0 {
		t.Fatalf("aliases = %v, want the junk alias dropped", stored.Aliases)
	}

	again, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("reload service twice: %v", err)
	}
	if len(again.ListGames()) != 1 {
		t.Fatalf("games = %d, want 1", len(again.ListGames()))
	}
	if _, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
		t.Fatalf("catalog file: %v", err)
	}
}
