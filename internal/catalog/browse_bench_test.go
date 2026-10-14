package catalog

import (
	"context"
	"fmt"
	"testing"
)

func benchRemoteGame(i int) Game {
	title := benchTitle(1, i)
	g := Game{
		ID:          fmt.Sprintf("g%06d", i),
		ServerID:    fmt.Sprintf("srv%06d", i),
		Title:       title,
		SortTitle:   sortTitle(title),
		GameType:    "Main Game",
		ExternalIDs: ExternalIDs{IGDB: fmt.Sprintf("%d", 100000+i)},
		ProviderLinks: map[string][]string{
			"igdb": {fmt.Sprintf("%d", 100000+i)},
		},
	}
	if i%2 == 0 {
		g.ExternalIDs.Steam = fmt.Sprintf("%d", 500000+i)
		g.ProviderLinks["steam"] = []string{g.ExternalIDs.Steam}
	}
	return g
}

func benchRemoteCatalog(tb testing.TB, size int) *Service {
	tb.Helper()
	s, err := NewServiceAt(tb.TempDir())
	if err != nil {
		tb.Fatalf("new catalog: %v", err)
	}
	games := make([]Game, 0, size)
	for i := range size {
		games = append(games, benchRemoteGame(i))
	}
	s.games = games
	s.rebuildLocked()
	return s
}

func benchRemotePage(size, items, overlap int) GamePage {
	page := GamePage{Total: size, Page: 1, PageSize: items, Revision: 1}
	for i := range overlap {
		g := benchRemoteGame((i*613 + 7) % size)
		g.ID = g.ServerID
		g.ServerID = ""
		page.Items = append(page.Items, g)
	}
	for i := overlap; i < items; i++ {
		title := benchTitle(3, i)
		page.Items = append(page.Items, Game{
			ID:          fmt.Sprintf("fresh%04d", i),
			Title:       title,
			SortTitle:   sortTitle(title),
			GameType:    "Main Game",
			ExternalIDs: ExternalIDs{IGDB: fmt.Sprintf("%d", 900000+i)},
		})
	}
	return page
}

func BenchmarkBrowsePage40k(b *testing.B) {
	const size = 40000
	s := benchRemoteCatalog(b, size)
	s.SetRemoteCatalog(&remoteFixture{page: benchRemotePage(size, 60, 40)})
	q := GameQuery{Page: 1, PageSize: 60}
	if _, err := s.BrowseGames(context.Background(), q); err != nil {
		b.Fatalf("warm-up browse: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		page, err := s.BrowseGames(context.Background(), q)
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Items) != 60 {
			b.Fatalf("page items = %d, want 60", len(page.Items))
		}
	}
}
