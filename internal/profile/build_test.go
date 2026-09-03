package profile

import (
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/playlog"
)

var now = time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return now.Add(d) }

func ptr(t time.Time) *time.Time { return &t }

func games() []library.Game {
	return []library.Game{
		{ID: "wow", Title: "World of Warcraft", Cover: "wow.jpg", PlaytimeSeconds: 400 * 3600, Favorite: true, LastPlayed: ptr(at(-time.Hour))},
		{ID: "cp", Title: "Cyberpunk 2077", Cover: "cp.jpg", PlaytimeSeconds: 120 * 3600, Status: library.StatusCompleted, StatusAt: ptr(at(-48 * time.Hour)), Favorite: true, LastPlayed: ptr(at(-30 * time.Hour))},
		{ID: "terraria", Title: "Terraria", PlaytimeSeconds: 3*3600 + 41*60, Status: library.StatusCompleted, StatusAt: ptr(at(-100 * 24 * time.Hour)), Uninstalled: true},
		{ID: "idle", Title: "Never Played", PlaytimeSeconds: 0},
	}
}

func sessions() []playlog.Session {
	return []playlog.Session{
		{GameID: "wow", StartedAt: at(-3 * time.Hour), EndedAt: at(-46 * time.Minute)},
		{GameID: "cp", StartedAt: at(-30 * time.Hour), EndedAt: at(-29 * time.Hour)},
		{GameID: "terraria", StartedAt: at(-72 * time.Hour), EndedAt: at(-72*time.Hour + 3*time.Hour + 41*time.Minute)},
		{GameID: "wow", StartedAt: at(-15 * 24 * time.Hour), EndedAt: at(-13 * 24 * time.Hour)},
		{GameID: "ghost", StartedAt: at(-2 * time.Hour), EndedAt: at(-time.Hour)},
		{GameID: "cp", StartedAt: at(-20 * 24 * time.Hour), EndedAt: at(-19 * 24 * time.Hour)},
	}
}

func TestBuildStats(t *testing.T) {
	snap := Build(games(), sessions(), nil, []string{"favorites"}, now)
	if snap.Stats.Games != 4 {
		t.Errorf("Games = %d, want 4 (uninstalled still counts)", snap.Stats.Games)
	}
	if snap.Stats.Hours != 523 {
		t.Errorf("Hours = %d, want 523", snap.Stats.Hours)
	}
	if snap.Stats.Completed != 2 {
		t.Errorf("Completed = %d, want 2", snap.Stats.Completed)
	}
	if snap.Stats.Playing != 3 {
		t.Errorf("Playing = %d, want 3 (wow, cp, terraria in 14 days; ghost unknown)", snap.Stats.Playing)
	}
}

func TestBuildPlayingWindowIsClipped(t *testing.T) {
	snap := Build(games(), sessions(), nil, nil, now)
	if len(snap.Playing) != 3 || snap.Playing[0].Game.ID != "wow" {
		t.Fatalf("Playing = %+v, want wow first", snap.Playing)
	}
	wantWow := int64((2*time.Hour + 14*time.Minute + 24*time.Hour).Seconds())
	if snap.Playing[0].RecentSeconds != wantWow {
		t.Errorf("wow RecentSeconds = %d, want %d (2h14m now + 24h clipped tail of the old session)", snap.Playing[0].RecentSeconds, wantWow)
	}
	if snap.Playing[1].Game.ID != "terraria" || snap.Playing[2].Game.ID != "cp" {
		t.Errorf("order = %s, %s; want terraria, cp", snap.Playing[1].Game.ID, snap.Playing[2].Game.ID)
	}
}

func TestBuildActivityGroupsByLocalDay(t *testing.T) {
	loc := time.FixedZone("plus3", 3*3600)
	snap := Build(games(), sessions(), nil, nil, now.In(loc))
	if len(snap.Activity) != 4 {
		t.Fatalf("days = %d, want 4: %+v", len(snap.Activity), snap.Activity)
	}
	if snap.Activity[0].Date != "2026-09-03" || snap.Activity[0].Entries[0].Game.ID != "wow" {
		t.Errorf("day 0 = %+v, want today/wow", snap.Activity[0])
	}
	if snap.Activity[0].Entries[0].Seconds != int64((2*time.Hour + 14*time.Minute).Seconds()) {
		t.Errorf("today wow seconds = %d", snap.Activity[0].Entries[0].Seconds)
	}
	if snap.Activity[1].Date != "2026-09-02" || snap.Activity[1].Entries[0].Game.ID != "cp" {
		t.Errorf("day 1 = %+v, want yesterday/cp", snap.Activity[1])
	}
	if snap.Activity[2].Date != "2026-08-31" || snap.Activity[2].Entries[0].Game.ID != "terraria" {
		t.Errorf("day 2 = %+v, want Aug 31/terraria", snap.Activity[2])
	}
	if snap.Activity[3].Date != "2026-08-21" {
		t.Errorf("day 3 = %+v, want the clipped tail of the old wow session on Aug 21", snap.Activity[3])
	}
}

func TestBuildRunningAndShowcase(t *testing.T) {
	snap := Build(games(), sessions(), []string{"cp", "ghost"}, []string{"most_played", "recently_completed", "favorites"}, now)
	if len(snap.Running) != 1 || snap.Running[0].Title != "Cyberpunk 2077" {
		t.Fatalf("Running = %+v, want cp only", snap.Running)
	}
	if len(snap.Showcase) != 3 {
		t.Fatalf("Showcase = %+v", snap.Showcase)
	}
	mp := snap.Showcase[0]
	if mp.Kind != "most_played" || len(mp.Games) != 3 || mp.Games[0].ID != "wow" || mp.Games[2].ID != "terraria" {
		t.Errorf("most_played = %+v", mp)
	}
	rc := snap.Showcase[1]
	if rc.Kind != "recently_completed" || len(rc.Games) != 2 || rc.Games[0].ID != "cp" {
		t.Errorf("recently_completed = %+v", rc)
	}
	fav := snap.Showcase[2]
	if fav.Kind != "favorites" || len(fav.Games) != 2 || fav.Games[0].ID != "wow" {
		t.Errorf("favorites = %+v", fav)
	}
}

func TestBuildEmptyIsNotNil(t *testing.T) {
	snap := Build(nil, nil, nil, []string{"favorites"}, now)
	if snap.Playing == nil || snap.Activity == nil || snap.Running == nil || snap.Showcase == nil {
		t.Fatalf("slices must be empty, not nil: %+v", snap)
	}
	if len(snap.Showcase) != 1 || snap.Showcase[0].Games == nil {
		t.Fatalf("empty showcase block must still be present with an empty list: %+v", snap.Showcase)
	}
}
