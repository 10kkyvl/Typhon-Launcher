package library

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRecentGames(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time {
		v := base.Add(time.Duration(minutes) * time.Minute)
		return &v
	}
	cases := []struct {
		name     string
		games    []Game
		running  []string
		starting []string
		limit    int
		want     []RecentGame
	}{
		{"empty library", nil, nil, nil, 5, []RecentGame{}},
		{"zero limit", []Game{{ID: "a", Title: "A", LastPlayed: at(1)}}, nil, nil, 0, nil},
		{"negative limit", []Game{{ID: "a", Title: "A", LastPlayed: at(1)}}, nil, nil, -1, nil},
		{
			"never played is skipped",
			[]Game{{ID: "a", Title: "A"}, {ID: "b", Title: "B", LastPlayed: at(1)}},
			nil, nil, 5,
			[]RecentGame{{ID: "b", Title: "B"}},
		},
		{
			"newest first",
			[]Game{
				{ID: "a", Title: "A", Executable: "a.exe", LastPlayed: at(1)},
				{ID: "b", Title: "B", LastPlayed: at(30)},
				{ID: "c", Title: "C", LastPlayed: at(10)},
			},
			nil, nil, 5,
			[]RecentGame{{ID: "b", Title: "B"}, {ID: "c", Title: "C"}, {ID: "a", Title: "A", Executable: "a.exe"}},
		},
		{
			"hidden uninstalled and archived are skipped",
			[]Game{
				{ID: "h", Title: "H", Hidden: true, LastPlayed: at(50)},
				{ID: "u", Title: "U", Uninstalled: true, LastPlayed: at(40)},
				{ID: "r", Title: "R", Archived: true, LastPlayed: at(30)},
				{ID: "ok", Title: "OK", LastPlayed: at(1)},
			},
			nil, nil, 5,
			[]RecentGame{{ID: "ok", Title: "OK"}},
		},
		{
			"limit cuts the oldest",
			[]Game{
				{ID: "a", Title: "A", LastPlayed: at(1)},
				{ID: "b", Title: "B", LastPlayed: at(2)},
				{ID: "c", Title: "C", LastPlayed: at(3)},
			},
			nil, nil, 2,
			[]RecentGame{{ID: "c", Title: "C"}, {ID: "b", Title: "B"}},
		},
		{
			"equal times fall back to id",
			[]Game{
				{ID: "b", Title: "B", LastPlayed: at(1)},
				{ID: "a", Title: "A", LastPlayed: at(1)},
			},
			nil, nil, 5,
			[]RecentGame{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}},
		},
		{
			"running and starting are flagged",
			[]Game{
				{ID: "a", Title: "A", LastPlayed: at(3)},
				{ID: "b", Title: "B", LastPlayed: at(2)},
				{ID: "c", Title: "C", LastPlayed: at(1)},
			},
			[]string{"a"}, []string{"b"}, 5,
			[]RecentGame{{ID: "a", Title: "A", Running: true}, {ID: "b", Title: "B", Running: true}, {ID: "c", Title: "C"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			s.mu.Lock()
			s.games = tc.games
			s.starting = map[string]context.CancelFunc{}
			for _, id := range tc.running {
				s.running[id] = &session{}
			}
			for _, id := range tc.starting {
				s.starting[id] = func() {}
			}
			s.mu.Unlock()

			got := s.RecentGames(tc.limit)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RecentGames(%d) = %+v, want %+v", tc.limit, got, tc.want)
			}
		})
	}
}

func TestRecentGamesDoesNotAliasLibrary(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	played := time.Now()
	s.mu.Lock()
	s.games = []Game{{ID: "a", Title: "A", LastPlayed: &played}}
	s.mu.Unlock()

	got := s.RecentGames(5)
	got[0].Title = "changed"
	if s.GetGames()[0].Title != "A" {
		t.Fatal("RecentGames result shares memory with the library")
	}
}

// Сессия из s.running удалена до persist, поэтому ошибка записи не должна
// оставлять UI и трей в состоянии «Играет».
func TestFinishSessionEmitsStoppedWhenPersistFails(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	startedAt := time.Now().Add(-time.Minute)
	s.mu.Lock()
	s.games = []Game{{ID: "g", Title: "G", PlaytimeSeconds: 100}}
	s.running["g"] = &session{startedAt: startedAt, lastSeen: startedAt}
	s.mu.Unlock()
	s.path = t.TempDir() // atomic replacement of a directory must fail

	stopped := make(chan SessionEvent, 1)
	prev := emitStopped
	emitStopped = func(ev SessionEvent) { stopped <- ev }
	t.Cleanup(func() { emitStopped = prev })
	updates := 0
	s.onUpdated = func([]Game) { updates++ }

	s.finishSession("g", startedAt)

	select {
	case ev := <-stopped:
		if ev.GameID != "g" || ev.PlaytimeSeconds != 100 {
			t.Fatalf("stopped event = %+v, want game g with the rolled-back playtime 100", ev)
		}
	default:
		t.Fatal("game:stopped was not emitted after the persist failure")
	}
	if updates != 1 {
		t.Fatalf("library:updated emitted %d times, want 1", updates)
	}
	if s.IsRunning("g") {
		t.Fatal("session still running after finishSession")
	}
	games := s.GetGames()
	if games[0].LastPlayed != nil || games[0].PlaytimeSeconds != 100 {
		t.Fatalf("game = %+v, want the in-memory state rolled back", games[0])
	}
}
