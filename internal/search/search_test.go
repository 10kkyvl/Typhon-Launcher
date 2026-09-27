package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/sources"
)

var errNoGame = errors.New("game not found")

type fakeLibrary struct {
	games []library.Game
}

func (f fakeLibrary) GetInstalledGames() []library.Game {
	return f.games
}

type fakeCatalog struct {
	games []catalog.Game
}

func (f fakeCatalog) BrowseGames(q catalog.GameQuery) (catalog.GamePage, error) {
	needle := strings.ToLower(q.Search)
	out := make([]catalog.Game, 0, len(f.games))
	for _, game := range f.games {
		if contains(game.Title, needle) || containsAny(game.Aliases, needle) {
			out = append(out, game)
		}
	}
	total := len(out)
	if q.PageSize > 0 && len(out) > q.PageSize {
		out = out[:q.PageSize]
	}
	return catalog.GamePage{Items: out, Total: total}, nil
}

func (f fakeCatalog) GetGame(id string) (catalog.Game, error) {
	for _, game := range f.games {
		if game.ID == id {
			return game, nil
		}
	}
	return catalog.Game{}, errNoGame
}

func contains(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), needle)
}

func containsAny(values []string, needle string) bool {
	for _, value := range values {
		if contains(value, needle) {
			return true
		}
	}
	return false
}

type fakeSources struct {
	matches sources.ReleaseMatches
	queries []string
	gameIDs [][]string
}

func (f *fakeSources) SearchReleaseMatches(query string, gameIDs []string, _ int) sources.ReleaseMatches {
	f.queries = append(f.queries, query)
	f.gameIDs = append(f.gameIDs, gameIDs)
	return f.matches
}

func noReleases() *fakeSources {
	return &fakeSources{matches: sources.ReleaseMatches{Games: map[string]sources.GameReleaseInfo{}}}
}

func withGames(games map[string]sources.GameReleaseInfo) *fakeSources {
	return &fakeSources{matches: sources.ReleaseMatches{Games: games}}
}

func unmatchedView(id, sourceID, sourceName, title string) sources.ReleaseView {
	return sources.ReleaseView{
		Release: sources.Release{
			ID:           id,
			SourceID:     sourceID,
			RawTitle:     title,
			Title:        title,
			MatchStatus:  catalog.StatusUnmatched,
			Availability: sources.AvailabilityAvailable,
		},
		SourceName: sourceName,
	}
}

func year(value int) *int {
	return &value
}

func TestEmptyQueryReturnsNothing(t *testing.T) {
	backend := withGames(map[string]sources.GameReleaseInfo{"game-1": {Releases: 3}})
	s := NewService(
		fakeLibrary{games: []library.Game{{ID: "lib-1", Title: "Cyberpunk 2077"}}},
		fakeCatalog{games: []catalog.Game{{ID: "game-1", Title: "Cyberpunk 2077"}}},
		backend,
	)

	for _, query := range []string{"", "   ", "\t\n"} {
		result := mustSearch(t, s, query)
		if len(result.Games) != 0 || len(result.Releases) != 0 {
			t.Fatalf("query %q must return nothing, got %+v", query, result)
		}
	}
	if len(backend.queries) != 0 {
		t.Fatalf("empty query must not reach the release index, got %v", backend.queries)
	}
}

func TestSingleCharacterQueryFindsTitlesButSkipsReleases(t *testing.T) {
	backend := noReleases()
	s := NewService(
		fakeLibrary{games: []library.Game{{ID: "lib-1", Title: "Prey", CanonicalGameID: "game-1"}}},
		fakeCatalog{games: []catalog.Game{{ID: "game-1", Title: "Prey"}}},
		backend,
	)

	result := mustSearch(t, s, "p")

	if len(result.Games) != 1 || result.Games[0].ID != "lib-1" {
		t.Fatalf("expected the installed game, got %+v", result.Games)
	}
	if len(backend.queries) != 0 {
		t.Fatalf("single character query must not scan releases, got %v", backend.queries)
	}
}

func TestExactCanonicalTitleSearch(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{games: []catalog.Game{{ID: "game-1", Title: "Cyberpunk 2077", ReleaseYear: year(2020)}}},
		noReleases(),
	)

	result := mustSearch(t, s, "Cyberpunk 2077")

	if len(result.Games) != 1 {
		t.Fatalf("expected one hit, got %d", len(result.Games))
	}
	hit := result.Games[0]
	if hit.ID != "game-1" || hit.Title != "Cyberpunk 2077" || hit.Year != 2020 {
		t.Fatalf("unexpected hit %+v", hit)
	}
	if hit.Score != scoreExactTitle {
		t.Fatalf("expected exact title score, got %v", hit.Score)
	}
}

func TestAliasSearchFindsCanonicalGame(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{games: []catalog.Game{{
			ID:      "game-1",
			Title:   "Cyberpunk 2077",
			Aliases: []string{"CP2077"},
		}}},
		noReleases(),
	)

	result := mustSearch(t, s, "cp2077")

	if len(result.Games) != 1 || result.Games[0].ID != "game-1" {
		t.Fatalf("expected alias hit, got %+v", result.Games)
	}
	if result.Games[0].Score != scoreExactAlias {
		t.Fatalf("expected exact alias score, got %v", result.Games[0].Score)
	}
}

func TestMatchedReleasesAggregateUnderCanonicalGame(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{games: []catalog.Game{{ID: "game-1", Title: "Cyberpunk 2077"}}},
		withGames(map[string]sources.GameReleaseInfo{
			"game-1": {Title: "Cyberpunk 2077", Releases: 4, Sources: 3, LatestVersion: "2.31"},
		}),
	)

	result := mustSearch(t, s, "cyberpunk")

	if len(result.Games) != 1 {
		t.Fatalf("expected one game hit, got %d", len(result.Games))
	}
	hit := result.Games[0]
	if hit.Releases != 4 || hit.Sources != 3 || hit.LatestVersion != "2.31" {
		t.Fatalf("expected aggregated release data, got %+v", hit)
	}
	if len(result.Releases) != 0 {
		t.Fatalf("matched releases must not appear separately, got %d", len(result.Releases))
	}
}

func TestReleaseOnlyGameIsCreatedWithoutDuplicates(t *testing.T) {
	backend := withGames(map[string]sources.GameReleaseInfo{
		"game-1": {Title: "Cyberpunk 2077 Ultimate Edition", Releases: 2, Sources: 1},
	})
	s := NewService(fakeLibrary{}, fakeCatalog{}, backend)

	result := mustSearch(t, s, "cyberpunk")

	if len(result.Games) != 0 {
		t.Fatalf("local entries leaked into server search: %+v", result)
	}
}

func TestInstalledGameMergesWithCanonicalHit(t *testing.T) {
	backend := withGames(map[string]sources.GameReleaseInfo{
		"game-1": {Title: "Cyberpunk 2077", Releases: 3, Sources: 2, LatestVersion: "2.31"},
	})
	s := NewService(
		fakeLibrary{games: []library.Game{{
			ID:              "lib-1",
			Title:           "Cyberpunk 2077",
			Version:         "2.2",
			Cover:           "cover.jpg",
			CanonicalGameID: "game-1",
		}}},
		fakeCatalog{games: []catalog.Game{{ID: "game-1", Title: "Cyberpunk 2077"}}},
		backend,
	)

	result := mustSearch(t, s, "cyberpunk")

	if len(result.Games) != 1 {
		t.Fatalf("expected one merged hit, got %+v", result.Games)
	}
	hit := result.Games[0]
	if !hit.Installed {
		t.Fatal("expected hit to be marked installed")
	}
	if hit.ID != "lib-1" {
		t.Fatalf("installed hit must route to the library game, got %q", hit.ID)
	}
	if hit.CanonicalGameID != "game-1" || hit.Version != "2.2" || hit.Cover != "cover.jpg" {
		t.Fatalf("unexpected merged hit %+v", hit)
	}
	if hit.Releases != 3 || hit.LatestVersion != "2.31" {
		t.Fatalf("expected release aggregate on the installed hit, got %+v", hit)
	}
	if len(backend.gameIDs) != 1 || len(backend.gameIDs[0]) != 1 || backend.gameIDs[0][0] != "game-1" {
		t.Fatalf("expected canonical id forwarded to the release index, got %v", backend.gameIDs)
	}
}

func TestInstalledGameMergesWithReleaseOnlyHit(t *testing.T) {
	s := NewService(
		fakeLibrary{games: []library.Game{{
			ID:              "lib-1",
			Title:           "CP2077",
			Version:         "2.2",
			CanonicalGameID: "game-1",
		}}},
		fakeCatalog{},
		withGames(map[string]sources.GameReleaseInfo{
			"game-1": {Title: "Cyberpunk 2077", Releases: 1, Sources: 1},
		}),
	)

	result := mustSearch(t, s, "cyberpunk")

	if len(result.Games) != 0 {
		t.Fatalf("local entries leaked into server search: %+v", result)
	}
}

func TestInstalledGameWithoutCanonicalIDIsFoundByTitle(t *testing.T) {
	s := NewService(
		fakeLibrary{games: []library.Game{{ID: "lib-1", Title: "Workers & Resources Soviet Republic"}}},
		fakeCatalog{},
		noReleases(),
	)

	result := mustSearch(t, s, "workers")

	if len(result.Games) != 0 {
		t.Fatalf("local entries leaked into server search: %+v", result)
	}
}

func TestUnmatchedReleasesStayInOwnSection(t *testing.T) {
	backend := &fakeSources{matches: sources.ReleaseMatches{
		Games:         map[string]sources.GameReleaseInfo{},
		Unmatched:     []sources.ReleaseView{unmatchedView("r1", "src-1", "Feed A", "Some Game Build 1234")},
		MoreUnmatched: 2,
	}}
	s := NewService(fakeLibrary{}, fakeCatalog{}, backend)

	result := mustSearch(t, s, "Some Game")

	if len(result.Games) != 0 || len(result.Releases) != 0 || result.MoreReleases != 0 {
		t.Fatalf("source entries leaked into search: %+v", result)
	}
}

func TestAmbiguousCanonicalGamesStaySeparate(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{games: []catalog.Game{
			{ID: "prey-2006", Title: "Prey", ReleaseYear: year(2006)},
			{ID: "prey-2017", Title: "Prey", ReleaseYear: year(2017)},
		}},
		noReleases(),
	)

	result := mustSearch(t, s, "Prey")

	if len(result.Games) != 2 {
		t.Fatalf("expected both games, got %+v", result.Games)
	}
	if result.Games[0].ID == result.Games[1].ID {
		t.Fatal("games must not be collapsed into one hit")
	}
}

func TestRankingOrder(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{games: []catalog.Game{
			{ID: "substring", Title: "The Doom Chronicles"},
			{ID: "prefix", Title: "Doom Eternal"},
			{ID: "alias", Title: "Ultimate Slayer", Aliases: []string{"Doom"}},
			{ID: "exact", Title: "DOOM"},
		}},
		withGames(map[string]sources.GameReleaseInfo{
			"release-only": {Title: "Doom Repack", Releases: 1, Sources: 1},
		}),
	)

	result := mustSearch(t, s, "doom")

	got := make([]string, 0, len(result.Games))
	for _, hit := range result.Games {
		got = append(got, hit.ID)
	}
	want := []string{"exact", "alias", "prefix", "substring"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestInstalledDoesNotOutrankBetterTitleMatch(t *testing.T) {
	s := NewService(
		fakeLibrary{games: []library.Game{{ID: "lib-1", Title: "The Doom Chronicles"}}},
		fakeCatalog{games: []catalog.Game{{ID: "exact", Title: "DOOM"}}},
		noReleases(),
	)

	result := mustSearch(t, s, "doom")

	if len(result.Games) != 1 {
		t.Fatalf("expected only server hit, got %+v", result.Games)
	}
	if result.Games[0].ID != "exact" {
		t.Fatalf("exact title must outrank the installed substring match, got %q", result.Games[0].ID)
	}
}

func TestRankingIsDeterministic(t *testing.T) {
	games := []catalog.Game{
		{ID: "game-c", Title: "Test Game"},
		{ID: "game-a", Title: "Test Game"},
		{ID: "game-b", Title: "Test Game"},
	}
	s := NewService(fakeLibrary{}, fakeCatalog{games: games}, noReleases())

	first := mustSearch(t, s, "test game")
	for range 10 {
		next := mustSearch(t, s, "test game")
		if len(next.Games) != len(first.Games) {
			t.Fatalf("result size changed: %d vs %d", len(next.Games), len(first.Games))
		}
		for i := range next.Games {
			if next.Games[i].ID != first.Games[i].ID {
				t.Fatalf("order changed at %d: %q vs %q", i, next.Games[i].ID, first.Games[i].ID)
			}
		}
	}
	if first.Games[0].ID != "game-a" {
		t.Fatalf("expected stable tie-break by id, got %q", first.Games[0].ID)
	}
}

func TestResultsAreLimited(t *testing.T) {
	games := make([]catalog.Game, 0, maxGames+2)
	for i := range maxGames + 2 {
		games = append(games, catalog.Game{ID: "game-" + string(rune('a'+i)), Title: "Test Game"})
	}
	unmatched := make([]sources.ReleaseView, 0, maxReleases)
	for i := range maxReleases {
		unmatched = append(unmatched, unmatchedView(string(rune('a'+i)), "src-1", "Feed A", "Test Game"))
	}
	backend := &fakeSources{matches: sources.ReleaseMatches{
		Games:         map[string]sources.GameReleaseInfo{},
		Unmatched:     unmatched,
		MoreUnmatched: 3,
	}}
	s := NewService(fakeLibrary{}, fakeCatalog{games: games}, backend)

	result := mustSearch(t, s, "test")

	if len(result.Games) != maxGames || result.MoreGames != 2 {
		t.Fatalf("expected %d games and 2 more, got %d and %d", maxGames, len(result.Games), result.MoreGames)
	}
	if len(result.Releases) != 0 || result.MoreReleases != 0 {
		t.Fatalf("expected %d releases and 3 more, got %d and %d", maxReleases, len(result.Releases), result.MoreReleases)
	}
}

func TestMissingCatalogMetadataFallsBackToReleaseTitle(t *testing.T) {
	s := NewService(
		fakeLibrary{},
		fakeCatalog{},
		withGames(map[string]sources.GameReleaseInfo{
			"game-1": {Title: "", Releases: 1, Sources: 1},
		}),
	)

	result := mustSearch(t, s, "cyberpunk")

	if len(result.Games) != 0 {
		t.Fatalf("local entries leaked into server search: %+v", result)
	}
}

func TestUnavailableCatalogDoesNotFallBackToLocalEntries(t *testing.T) {
	s := NewService(fakeLibrary{games: []library.Game{{ID: "local", Title: "Cyberpunk"}}}, nil, noReleases())
	result, err := s.Search("cyberpunk")
	if err == nil || len(result.Games) != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func mustSearch(t *testing.T, s *Service, query string) Result {
	t.Helper()
	result, err := s.Search(query)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// Exercise the real catalog boundary: personal cached rows must never become
// search membership, even while server results are persisted into that cache.
func TestSearchUsesServerCatalogAndReportsFailure(t *testing.T) {
	cat, err := catalog.NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.EnsureGame("GTA local repack", 0); err != nil {
		t.Fatal(err)
	}
	remote := &searchRemote{}
	cat.SetRemoteCatalog(remote)
	s := NewService(nil, cat, withGames(map[string]sources.GameReleaseInfo{"local": {Title: "GTA source repack", Releases: 2}}))
	got := mustSearch(t, s, "gta")
	if len(got.Games) != 1 || got.Games[0].Title != "Grand Theft Auto V" || got.Games[0].Cover != "https://example.com/cover.jpg" {
		t.Fatalf("unexpected server result: %+v", got)
	}
	if remote.query.Search != "gta" || remote.query.Kind != "all" {
		t.Fatalf("query=%+v", remote.query)
	}
	remote.err = errors.New("server unavailable")
	got = mustSearch(t, s, "gta") // exact server-page cache remains valid offline
	if len(got.Games) != 1 {
		t.Fatalf("offline page: %+v", got)
	}
	if _, err := s.Search("uncached"); err == nil {
		t.Fatal("server failure hidden")
	}
}

type searchRemote struct {
	query catalog.GameQuery
	err   error
}

func (r *searchRemote) Browse(_ context.Context, q catalog.GameQuery) (catalog.GamePage, error) {
	r.query = q
	return catalog.GamePage{Items: []catalog.Game{{ID: "official", Title: "Grand Theft Auto V", Aliases: []string{"GTA V"}, CoverURL: "https://example.com/cover.jpg"}}, Total: 1}, r.err
}
