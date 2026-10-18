package catalog

import (
	"testing"
	"time"
)

func sharesBacking(a, b []Game) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

func TestRecommendationSnapshotIsCachedUntilTheCatalogOrPreferencesMove(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "First", Genres: []string{"Action"}},
		Game{Title: "Second", ServerID: "second-server", Genres: []string{"Puzzle"}},
	)
	first, _, _ := s.recommendationSnapshot()
	if len(first) != 2 {
		t.Fatalf("snapshot games = %d, want 2", len(first))
	}

	cases := []struct {
		name       string
		act        func(t *testing.T)
		wantSame   bool
		wantGames  int
		wantNotInt []string
	}{
		{name: "nothing changed", act: func(*testing.T) {}, wantSame: true, wantGames: 2},
		{name: "an unrelated read", act: func(*testing.T) { s.GetRecommendationPreferences() }, wantSame: true, wantGames: 2},
		{name: "a game is added", act: func(t *testing.T) {
			seed(t, s, Game{Title: "Third"})
		}, wantGames: 3},
		{name: "metadata is applied", act: func(t *testing.T) {
			if _, err := s.ApplyMetadata(games[0].ID, MetadataPatch{IGDBID: "42", Title: "First", UpdatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}, wantGames: 3},
		{name: "a game is dismissed", act: func(t *testing.T) {
			if err := s.SetNotInterested("second-server", true); err != nil {
				t.Fatal(err)
			}
		}, wantGames: 3, wantNotInt: []string{games[1].ID}},
		{name: "the dismissal is undone", act: func(t *testing.T) {
			if err := s.SetNotInterested(games[1].ID, false); err != nil {
				t.Fatal(err)
			}
		}, wantGames: 3},
		{name: "preferences are saved", act: func(t *testing.T) {
			p := s.GetRecommendationPreferences()
			p.DefaultSort = "popular"
			if err := s.SaveRecommendationPreferences(p); err != nil {
				t.Fatal(err)
			}
		}, wantGames: 3},
	}
	previous := first
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.act(t)
			got, p, _ := s.recommendationSnapshot()
			if same := sharesBacking(previous, got); same != tc.wantSame {
				t.Fatalf("snapshot reused = %v, want %v", same, tc.wantSame)
			}
			if len(got) != tc.wantGames {
				t.Fatalf("snapshot games = %d, want %d", len(got), tc.wantGames)
			}
			if len(p.NotInterested) != len(tc.wantNotInt) {
				t.Fatalf("not interested = %v, want %v", p.NotInterested, tc.wantNotInt)
			}
			for i := range tc.wantNotInt {
				if p.NotInterested[i] != tc.wantNotInt[i] {
					t.Fatalf("not interested = %v, want %v", p.NotInterested, tc.wantNotInt)
				}
			}
			previous = got
		})
	}
}

func TestRecommendationSnapshotCallersCannotCorruptTheCache(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{Title: "First"}, Game{Title: "Second"})
	first, p, _ := s.recommendationSnapshot()
	if cap(first) != len(first) {
		t.Fatalf("snapshot capacity %d exceeds its length %d, an append would write into the cache", cap(first), len(first))
	}
	_ = append(first, Game{ID: "intruder", Title: "Intruder"})
	p.NotInterested = append(p.NotInterested, "intruder")
	again, againPrefs, _ := s.recommendationSnapshot()
	if len(again) != 2 || len(againPrefs.NotInterested) != 0 {
		t.Fatalf("cache saw the caller's writes: %d games, not interested %v", len(again), againPrefs.NotInterested)
	}
}

func TestRecommendationSnapshotKeepsTheLibrarySourceLive(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Owned", ServerID: "owned-server"})
	var items []RecommendationLibraryItem
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem { return items })

	_, _, source := s.recommendationSnapshot()
	if got := source(); len(got) != 0 {
		t.Fatalf("empty library returned %v", got)
	}
	items = []RecommendationLibraryItem{{LibraryID: "lib", CanonicalGameID: "owned-server", Sessions: 3}}
	_, _, again := s.recommendationSnapshot()
	for name, src := range map[string]RecommendationLibrarySource{"snapshot taken before the change": source, "snapshot taken after": again} {
		got := src()
		if len(got) != 1 || got[0].CanonicalGameID != games[0].ID {
			t.Fatalf("%s: library items = %+v, want one item canonicalized to %s", name, got, games[0].ID)
		}
	}
}
