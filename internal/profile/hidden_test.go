package profile

import (
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/playlog"
)

func hiddenGames() []library.Game {
	return []library.Game{
		{ID: "shown", Title: "Shown", PlaytimeSeconds: 3600, Favorite: true, Status: library.StatusCompleted, StatusAt: ptr(at(-time.Hour)), LastPlayed: ptr(at(-time.Hour))},
		{ID: "secret", Title: "Secret", PlaytimeSeconds: 7200, Favorite: true, Status: library.StatusCompleted, StatusAt: ptr(at(-time.Minute)), LastPlayed: ptr(at(-time.Minute)), Hidden: true},
	}
}

func hiddenSessions() []playlog.Session {
	return []playlog.Session{
		{GameID: "shown", StartedAt: at(-3 * time.Hour), EndedAt: at(-2 * time.Hour)},
		{GameID: "secret", StartedAt: at(-2 * time.Hour), EndedAt: at(-time.Minute)},
	}
}

func TestBuildLeavesHiddenGamesOutOfShowcase(t *testing.T) {
	snap := Build(hiddenGames(), hiddenSessions(), nil, []string{"favorites", "recently_completed", "most_played"}, now)
	for _, block := range snap.Showcase {
		if len(block.Games) != 1 || block.Games[0].ID != "shown" {
			t.Errorf("%s = %+v, want only the visible game", block.Kind, block.Games)
		}
	}
}

func TestBuildLeavesHiddenGamesOutOfRecentlyPlayed(t *testing.T) {
	snap := Build(hiddenGames(), hiddenSessions(), []string{"secret", "shown"}, nil, now)
	if len(snap.Playing) != 1 || snap.Playing[0].Game.ID != "shown" {
		t.Errorf("Playing = %+v", snap.Playing)
	}
	for _, day := range snap.Activity {
		for _, e := range day.Entries {
			if e.Game.ID == "secret" {
				t.Errorf("Activity leaks a hidden game: %+v", day)
			}
		}
	}
	if len(snap.Running) != 1 || snap.Running[0].ID != "shown" {
		t.Errorf("Running = %+v", snap.Running)
	}
	if snap.Stats.Playing != 1 || snap.Stats.MonthGames != 1 {
		t.Errorf("Stats = %+v, want one playing and one month game", snap.Stats)
	}
}

func TestSnapshotLeavesHiddenGamesOutOfGenresAndLayout(t *testing.T) {
	cat := stubCatalog{games: map[string]catalog.Game{
		"a": catalogGame("a", "1", "A", "Shooter"),
		"b": catalogGame("b", "2", "B", "Puzzle"),
	}}
	games := []library.Game{
		{ID: "ga", Title: "A", CanonicalGameID: "a", PlaytimeSeconds: 3600},
		{ID: "gb", Title: "B", CanonicalGameID: "b", PlaytimeSeconds: 3600, Hidden: true},
	}
	blocks := []account.LayoutBlock{block("b1", "pinned", `{"igdbId":2}`)}
	snap, err := newSnapshotService(t, games, cat, blocks).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Genres.Genres) != 1 || snap.Genres.Genres[0].Name != "Shooter" {
		t.Errorf("genres = %+v, want only Shooter", snap.Genres)
	}
	pinned := snap.LayoutGames["2"].Game
	if pinned == nil || pinned.ID != "b" {
		t.Errorf("layout game = %+v, want the catalog entry, not the hidden library game", snap.LayoutGames["2"])
	}
}
