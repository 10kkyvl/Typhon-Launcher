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
		{ID: "terraria", Title: "Terraria", PlaytimeSeconds: 3*3600 + 41*60, Status: library.StatusCompleted, StatusAt: ptr(at(-3 * 24 * time.Hour)), Uninstalled: true},
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
		{GameID: "idle", StartedAt: at(-63*time.Hour - 30*time.Minute), EndedAt: at(-62*time.Hour - 30*time.Minute)},
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
	if snap.Stats.Playing != 4 {
		t.Errorf("Playing = %d, want 4 (wow, cp, terraria, idle in 14 days; ghost unknown)", snap.Stats.Playing)
	}
}

func TestBuildPlayingWindowIsClipped(t *testing.T) {
	snap := Build(games(), sessions(), nil, nil, now)
	if len(snap.Playing) != 4 || snap.Playing[0].Game.ID != "wow" {
		t.Fatalf("Playing = %+v, want wow first", snap.Playing)
	}
	wantWow := int64((2*time.Hour + 14*time.Minute + 24*time.Hour).Seconds())
	if snap.Playing[0].RecentSeconds != wantWow {
		t.Errorf("wow RecentSeconds = %d, want %d (2h14m now + 24h clipped tail of the old session)", snap.Playing[0].RecentSeconds, wantWow)
	}
	if snap.Playing[1].Game.ID != "terraria" || snap.Playing[2].Game.ID != "cp" || snap.Playing[3].Game.ID != "idle" {
		t.Errorf("order = %s, %s, %s; want terraria, cp, idle", snap.Playing[1].Game.ID, snap.Playing[2].Game.ID, snap.Playing[3].Game.ID)
	}
	if snap.Playing[2].RecentSeconds != 3600 || snap.Playing[3].RecentSeconds != 3600 {
		t.Errorf("cp/idle RecentSeconds = %d/%d, want 3600/3600 (tied, cp wins the tie-break on title)", snap.Playing[2].RecentSeconds, snap.Playing[3].RecentSeconds)
	}
}

func TestBuildActivityGroupsByLocalDay(t *testing.T) {
	loc := time.FixedZone("plus3", 3*3600)
	snap := Build(games(), sessions(), nil, nil, now.In(loc))
	if len(snap.Activity) != 5 {
		t.Fatalf("days = %d, want 5: %+v", len(snap.Activity), snap.Activity)
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
	if snap.Activity[2].Date != "2026-09-01" || snap.Activity[2].Entries[0].Game.ID != "idle" {
		t.Errorf("day 2 = %+v, want Sep 1/idle (session crossing the midnight boundary)", snap.Activity[2])
	}
	if snap.Activity[2].Entries[0].Seconds != 3600 {
		t.Errorf("Sep 1 idle seconds = %d, want 3600 (this is the recent-window bucket, not month-clipped)", snap.Activity[2].Entries[0].Seconds)
	}
	if snap.Activity[3].Date != "2026-08-31" || snap.Activity[3].Entries[0].Game.ID != "terraria" {
		t.Errorf("day 3 = %+v, want Aug 31/terraria", snap.Activity[3])
	}
	if snap.Activity[4].Date != "2026-08-21" {
		t.Errorf("day 4 = %+v, want the clipped tail of the old wow session on Aug 21", snap.Activity[4])
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

func TestBuildMonth(t *testing.T) {
	snap := Build(games(), sessions(), nil, nil, now)
	if snap.Stats.MonthSeconds != 13440 {
		t.Errorf("MonthSeconds = %d, want 13440 (wow 8040s Sep 3 + cp 3600s Sep 2 + idle 1800s clipped to Sep 1 00:00; terraria Aug 31, wow's and cp's old sessions and ghost fall outside the month)", snap.Stats.MonthSeconds)
	}
	if snap.Stats.MonthGames != 3 {
		t.Errorf("MonthGames = %d, want 3 (wow, cp, idle)", snap.Stats.MonthGames)
	}
	if snap.Stats.MonthCompleted != 1 {
		t.Errorf("MonthCompleted = %d, want 1 (cp completed Sep 1 15:00 UTC; terraria completed Aug 31 15:00 UTC is last month)", snap.Stats.MonthCompleted)
	}
}

func TestRefCarriesStatus(t *testing.T) {
	snap := Build(games(), sessions(), []string{"cp"}, []string{"recently_completed"}, now)
	rc := snap.Showcase[0]
	if rc.Kind != "recently_completed" || len(rc.Games) == 0 || rc.Games[0].ID != "cp" {
		t.Fatalf("recently_completed = %+v, want cp first", rc)
	}
	if rc.Games[0].Status != "completed" {
		t.Errorf("recently_completed[0].Status = %q, want completed", rc.Games[0].Status)
	}
	wantStatusAt := at(-48 * time.Hour)
	if rc.Games[0].StatusAt == nil || !rc.Games[0].StatusAt.Equal(wantStatusAt) {
		t.Errorf("recently_completed[0].StatusAt = %v, want %v", rc.Games[0].StatusAt, wantStatusAt)
	}
	if len(snap.Running) != 1 || snap.Running[0].Status != "completed" || snap.Running[0].StatusAt == nil {
		t.Fatalf("Running[0] = %+v, want cp with status carried through ref()", snap.Running)
	}
}
