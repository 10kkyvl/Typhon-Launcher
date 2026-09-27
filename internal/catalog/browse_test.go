package catalog

import (
	"fmt"
	"testing"
	"time"
)

func seedMany(t *testing.T, s *Service, count int) {
	t.Helper()
	games := make([]Game, 0, count)
	for i := range count {
		games = append(games, Game{Title: fmt.Sprintf("Sample Game %03d", i), ReleaseYear: year(2000 + i%20)})
	}
	seed(t, s, games...)
}

func TestQueryGamesPagination(t *testing.T) {
	s := newTestService(t)
	seedMany(t, s, 130)

	cases := []struct {
		name  string
		query GameQuery
		items int
		total int
	}{
		{"first page", GameQuery{Page: 1, PageSize: 50}, 50, 130},
		{"last page", GameQuery{Page: 3, PageSize: 50}, 30, 130},
		{"past the end", GameQuery{Page: 9, PageSize: 50}, 0, 130},
		{"defaults", GameQuery{}, defaultPageSize, 130},
		{"negative page", GameQuery{Page: -4, PageSize: -1}, defaultPageSize, 130},
		{"oversized page", GameQuery{PageSize: 5000}, 130, 130},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := s.QueryGames(tc.query)
			if len(page.Items) != tc.items {
				t.Fatalf("items = %d, want %d", len(page.Items), tc.items)
			}
			if page.Total != tc.total {
				t.Fatalf("total = %d, want %d", page.Total, tc.total)
			}
			if page.Page < 1 || page.PageSize < 1 {
				t.Fatalf("page = %d, pageSize = %d", page.Page, page.PageSize)
			}
		})
	}
}

func TestQueryGamesPagesCoverEveryGameOnce(t *testing.T) {
	s := newTestService(t)
	seedMany(t, s, 75)

	seen := map[string]int{}
	for page := 1; page <= 3; page++ {
		for _, game := range s.QueryGames(GameQuery{Page: page, PageSize: 30}).Items {
			seen[game.ID]++
		}
	}
	if len(seen) != 75 {
		t.Fatalf("unique games = %d, want 75", len(seen))
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("game %s returned %d times", id, count)
		}
	}
}

func TestQueryGamesSearch(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Cyberpunk 2077", Aliases: []string{"CP2077"}},
		Game{Title: "The Witcher 3: Wild Hunt"},
		Game{Title: "Half-Life 2"},
	)

	cases := []struct {
		name   string
		search string
		want   []string
	}{
		{"substring", "witcher", []string{"The Witcher 3: Wild Hunt"}},
		{"case insensitive", "CYBERPUNK", []string{"Cyberpunk 2077"}},
		{"alias", "cp2077", []string{"Cyberpunk 2077"}},
		{"punctuation ignored", "half life", []string{"Half-Life 2"}},
		{"empty query keeps all", "", []string{"Cyberpunk 2077", "Half-Life 2", "The Witcher 3: Wild Hunt"}},
		{"whitespace only keeps all", "   ", []string{"Cyberpunk 2077", "Half-Life 2", "The Witcher 3: Wild Hunt"}},
		{"no match", "gothic", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := s.QueryGames(GameQuery{Search: tc.search})
			if page.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d", page.Total, len(tc.want))
			}
			for i, title := range tc.want {
				if page.Items[i].Title != title {
					t.Fatalf("item %d = %q, want %q", i, page.Items[i].Title, title)
				}
			}
		})
	}
}

func TestQueryGamesSort(t *testing.T) {
	s := newTestService(t)
	now := time.Now()
	seed(t, s,
		Game{Title: "Beta", ReleaseYear: year(2010), CreatedAt: now.Add(-2 * time.Hour)},
		Game{Title: "Alpha", CreatedAt: now.Add(-time.Hour)},
		Game{Title: "Gamma", ReleaseYear: year(2021), CreatedAt: now},
	)

	cases := []struct {
		mode string
		want []string
	}{
		{"", []string{"Alpha", "Beta", "Gamma"}},
		{"title", []string{"Alpha", "Beta", "Gamma"}},
		{"year", []string{"Gamma", "Beta", "Alpha"}},
		{"added", []string{"Gamma", "Alpha", "Beta"}},
	}

	for _, tc := range cases {
		t.Run("sort "+tc.mode, func(t *testing.T) {
			page := s.QueryGames(GameQuery{Sort: tc.mode})
			for i, title := range tc.want {
				if page.Items[i].Title != title {
					t.Fatalf("item %d = %q, want %q", i, page.Items[i].Title, title)
				}
			}
		})
	}
}

func TestQueryGamesEmptyCatalog(t *testing.T) {
	s := newTestService(t)
	page := s.QueryGames(GameQuery{Search: "anything"})
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("page = %+v", page)
	}
	if page.Items == nil {
		t.Fatal("items = nil, want empty slice")
	}
}

func TestQueryGamesGenre(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Elden Ring", Genres: []string{"RPG", "Action"}},
		Game{Title: "Baldur's Gate 3", Genres: []string{"RPG", "Tactical"}},
		Game{Title: "Counter-Strike 2", Genres: []string{"Shooter", "Tactical"}},
		Game{Title: "The Witcher 3: Wild Hunt", Genres: []string{"RPG", "Adventure"}},
		Game{Title: "Celeste", Genres: []string{"Platformer"}},
		Game{Title: "Hades", Genres: []string{"Roguelike", "Action"}},
		Game{Title: "Katana Zero", Genres: []string{"Hack and slash/Beat 'em up", "Indie"}},
		Game{Title: "Stardew Valley", Genres: []string{"Simulator"}},
	)

	cases := []struct {
		name  string
		genre string
		want  []string
	}{
		{
			"экшен",
			"Экшен",
			[]string{"Elden Ring", "Hades", "Katana Zero"},
		},
		{
			"ролевые",
			"Ролевые",
			[]string{"Baldur's Gate 3", "Elden Ring", "The Witcher 3: Wild Hunt"},
		},
		{
			"шутеры",
			"Шутеры",
			[]string{"Counter-Strike 2"},
		},
		{
			"приключения",
			"Приключения",
			[]string{"Celeste", "The Witcher 3: Wild Hunt"},
		},
		{
			"стратегии",
			"Стратегии",
			[]string{"Baldur's Gate 3", "Counter-Strike 2"},
		},
		{
			"инди",
			"Инди",
			[]string{"Katana Zero"},
		},
		{
			"case insensitive",
			"шутеры",
			[]string{"Counter-Strike 2"},
		},
		{
			"empty genre keeps all",
			"",
			[]string{
				"Baldur's Gate 3", "Celeste", "Counter-Strike 2", "Elden Ring",
				"Hades", "Katana Zero", "Stardew Valley", "The Witcher 3: Wild Hunt",
			},
		},
		{
			"unknown label matches nothing",
			"Гонки",
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := s.QueryGames(GameQuery{Genre: tc.genre, PageSize: maxPageSize})
			if page.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d (items=%v)", page.Total, len(tc.want), titlesOf(page.Items))
			}
			got := titlesOf(page.Items)
			for i, title := range tc.want {
				if got[i] != title {
					t.Fatalf("item %d = %q, want %q (got=%v)", i, got[i], title, got)
				}
			}
		})
	}
}

func titlesOf(games []Game) []string {
	out := make([]string, len(games))
	for i, g := range games {
		out[i] = g.Title
	}
	return out
}

func TestGenreFacetsCountsMatchFilteredResults(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Elden Ring", Genres: []string{"RPG", "Action"}},
		Game{Title: "Baldur's Gate 3", Genres: []string{"RPG", "Tactical"}},
		Game{Title: "Counter-Strike 2", Genres: []string{"Shooter", "Tactical"}},
		Game{Title: "The Witcher 3: Wild Hunt", Genres: []string{"RPG", "Adventure"}},
		Game{Title: "Celeste", Genres: []string{"Platformer"}},
		Game{Title: "Hades", Genres: []string{"Roguelike", "Action"}},
		Game{Title: "Katana Zero", Genres: []string{"Hack and slash/Beat 'em up", "Indie"}},
		Game{Title: "Stardew Valley", Genres: []string{"Simulator"}},
		Game{Title: "No Genre Game"},
	)

	facets := s.GenreFacets()
	if len(facets) != 6 {
		t.Fatalf("facets = %d, want 6", len(facets))
	}
	for _, facet := range facets {
		page := s.QueryGames(GameQuery{Genre: facet.Label, PageSize: maxPageSize})
		if facet.Count != page.Total {
			t.Fatalf("facet %q count = %d, want %d (matches QueryGames)", facet.Label, facet.Count, page.Total)
		}
		if facet.Count == 0 {
			t.Fatalf("facet %q count = 0, want > 0 given seeded data", facet.Label)
		}
	}
}

func TestGenreFacetsEmptyCatalog(t *testing.T) {
	s := newTestService(t)
	facets := s.GenreFacets()
	if len(facets) != 6 {
		t.Fatalf("facets = %d, want 6", len(facets))
	}
	for _, facet := range facets {
		if facet.Count != 0 {
			t.Fatalf("facet %q count = %d, want 0 on empty catalog", facet.Label, facet.Count)
		}
	}
}

func TestGetGames(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Alpha"},
		Game{Title: "Beta"},
	)

	cases := []struct {
		name string
		ids  []string
		want []string
	}{
		{"known ids keep order", []string{games[1].ID, games[0].ID}, []string{"Beta", "Alpha"}},
		{"unknown id skipped", []string{games[0].ID, "missing"}, []string{"Alpha"}},
		{"duplicates collapsed", []string{games[0].ID, games[0].ID}, []string{"Alpha"}},
		{"empty id skipped", []string{""}, nil},
		{"no ids", nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.GetGames(tc.ids)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d games, want %d", len(got), len(tc.want))
			}
			for i, title := range tc.want {
				if got[i].Title != title {
					t.Fatalf("item %d = %q, want %q", i, got[i].Title, title)
				}
			}
		})
	}
}
