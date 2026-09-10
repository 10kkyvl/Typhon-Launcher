package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/playlog"
)

func TestRemovedGameHistorySurvivesReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	if err := os.WriteFile(path, []byte(`[{"id":"game","title":"Historical game","cover":"cover.jpg","canonicalGameId":"42","playtimeSeconds":7200,"status":"completed"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	lib, err := library.NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	log, err := playlog.NewServiceAt(filepath.Join(dir, "playlog.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	log.Record("game", now.Add(-time.Hour), now)
	service := NewService(lib, log, func() []string { return []string{"most_played"} })
	before := service.Snapshot()
	if err := lib.RemoveGame("game"); err != nil {
		t.Fatal(err)
	}
	lib, err = library.NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	log, err = playlog.NewServiceAt(filepath.Join(dir, "playlog.json"))
	if err != nil {
		t.Fatal(err)
	}
	after := NewService(lib, log, func() []string { return []string{"most_played"} }).Snapshot()
	if len(lib.GetGames()) != 0 {
		t.Fatal("removed game returned to library")
	}
	if after.Stats.Games != 0 || after.Stats.Hours != before.Stats.Hours || after.Stats.Completed != before.Stats.Completed || after.Stats.MonthSeconds != before.Stats.MonthSeconds {
		t.Fatalf("before=%+v after=%+v", before.Stats, after.Stats)
	}
	if len(after.Activity) != 1 || len(after.Activity[0].Entries) != 1 {
		t.Fatalf("activity lost: %+v", after.Activity)
	}
	game := after.Activity[0].Entries[0].Game
	if game.Title != "Historical game" || game.Cover != "cover.jpg" || game.CanonicalGameID != "42" || !game.Archived {
		t.Fatalf("metadata lost: %+v", game)
	}
	if len(after.Showcase[0].Games) != 0 {
		t.Fatal("archived game in showcase")
	}
}
