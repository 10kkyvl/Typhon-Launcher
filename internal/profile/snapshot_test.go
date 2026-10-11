package profile

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/playlog"
)

type fixedLibrary struct{ games []library.Game }

func (l fixedLibrary) GetGames() []library.Game { return l.games }
func (fixedLibrary) GetRunningGames() []string  { return nil }

type emptyLog struct{}

func (emptyLog) Since(time.Time) []playlog.Session { return nil }

func catalogGame(id, igdb, title string, genres ...string) catalog.Game {
	return catalog.Game{ID: id, Title: title, CoverURL: title + ".jpg", Genres: genres, ExternalIDs: catalog.ExternalIDs{IGDB: igdb}}
}

func block(id, kind, config string) account.LayoutBlock {
	return account.LayoutBlock{ID: id, Type: kind, Width: "full", Config: json.RawMessage(config)}
}

func newSnapshotService(t *testing.T, games []library.Game, cat Catalog, blocks []account.LayoutBlock) *Service {
	t.Helper()
	s, err := NewService(fixedLibrary{games}, emptyLog{}, cat, func() []string { return nil }, func() []account.LayoutBlock { return blocks })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotGenresFromLibraryAndCatalog(t *testing.T) {
	cat := stubCatalog{games: map[string]catalog.Game{
		"a": catalogGame("a", "1", "A", "Role-playing (RPG)", "Adventure"),
		"b": catalogGame("b", "2", "B", "Shooter"),
		"c": catalogGame("c", "3", "C"),
		"d": catalogGame("d", "4", "D", "Puzzle"),
	}}
	games := []library.Game{
		{ID: "ga", CanonicalGameID: "a", PlaytimeSeconds: 3600, Status: library.StatusCompleted},
		{ID: "gb", CanonicalGameID: "b", PlaytimeSeconds: 1800},
		{ID: "gc", CanonicalGameID: "c", PlaytimeSeconds: 1800},
		{ID: "gd", CanonicalGameID: "d", PlaytimeSeconds: 0},
		{ID: "ge", CanonicalGameID: "gone-from-catalog", PlaytimeSeconds: 7200},
		{ID: "gf", PlaytimeSeconds: 0},
	}
	snap, err := newSnapshotService(t, games, cat, nil).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want := GenreBreakdown{
		Genres:  []GenreShare{{"Adventure", 0.125}, {"Role-playing (RPG)", 0.125}, {"Shooter", 0.125}},
		Unknown: 0.625,
	}
	if !reflect.DeepEqual(snap.Genres, want) {
		t.Fatalf("genres = %+v, want %+v", snap.Genres, want)
	}
	if snap.Fingerprint != (Fingerprint{Hours: 4, Games: 6, Completed: 1}) {
		t.Fatalf("fingerprint = %+v", snap.Fingerprint)
	}
	if snap.LayoutGames == nil || len(snap.LayoutGames) != 0 {
		t.Fatalf("layoutGames = %#v, want an empty non-nil map", snap.LayoutGames)
	}
}

func TestSnapshotLayoutGames(t *testing.T) {
	cat := stubCatalog{games: map[string]catalog.Game{
		"w3":  catalogGame("w3", "1942", "Witcher 3"),
		"hl":  catalogGame("hl", "7", "Half-Life"),
		"cat": catalogGame("cat", "9", "Only In Catalog"),
	}}
	completed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	games := []library.Game{
		{ID: "lib-w3", Title: "The Witcher 3", CanonicalGameID: "w3", PlaytimeSeconds: 7200, Status: library.StatusCompleted, StatusAt: &completed, Cover: "w3.png"},
		{ID: "lib-hl", Title: "Half-Life", CanonicalGameID: "hl", Archived: true, PlaytimeSeconds: 60},
		{ID: "lib-hl2", Title: "Half-Life", CanonicalGameID: "hl", PlaytimeSeconds: 120},
	}
	blocks := []account.LayoutBlock{
		block("b1", "pinned", `{"igdbId":1942,"caption":"best"}`),
		block("b2", "collection", `{"source":"manual","title":"mix","igdbIds":[7,9,4242,1942]}`),
		block("b3", "collection", `{"source":"favorites"}`),
		block("b4", "hologram", `{"anything":["goes"]}`),
		block("b5", "genres", `{}`),
	}
	snap, err := newSnapshotService(t, games, cat, blocks).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.LayoutGames) != 4 {
		t.Fatalf("layoutGames = %+v, want 1942, 7, 9, 4242", snap.LayoutGames)
	}
	w3 := snap.LayoutGames["1942"].Game
	if w3 == nil || w3.ID != "lib-w3" || w3.PlaytimeSeconds != 7200 || w3.Status != library.StatusCompleted || w3.Cover != "w3.png" {
		t.Fatalf("1942 = %+v, want the library entry", snap.LayoutGames["1942"])
	}
	hl := snap.LayoutGames["7"].Game
	if hl == nil || hl.ID != "lib-hl2" || hl.Archived {
		t.Fatalf("7 = %+v, want the non-archived library entry", snap.LayoutGames["7"])
	}
	only := snap.LayoutGames["9"].Game
	if only == nil || only.ID != "cat" || only.Title != "Only In Catalog" || only.Cover != "Only In Catalog.jpg" || only.PlaytimeSeconds != 0 || only.Status != "" {
		t.Fatalf("9 = %+v, want the catalog fallback", snap.LayoutGames["9"])
	}
	if missing := snap.LayoutGames["4242"]; missing.Game != nil || missing.Error != ErrUnresolved {
		t.Fatalf("4242 = %+v, want unresolved", missing)
	}
}

func TestSnapshotLayoutGamesRejectsBrokenConfig(t *testing.T) {
	cases := map[string]account.LayoutBlock{
		"pinned without config":       {ID: "b1", Type: "pinned", Width: "full"},
		"pinned with wrong type":      block("b1", "pinned", `{"igdbId":"1942"}`),
		"pinned zero id":              block("b1", "pinned", `{"igdbId":0}`),
		"pinned negative id":          block("b1", "pinned", `{"igdbId":-3}`),
		"collection not an object":    block("b1", "collection", `[1]`),
		"collection with bad id list": block("b1", "collection", `{"source":"manual","igdbIds":[1,"x"]}`),
		"collection with zero id":     block("b1", "collection", `{"source":"manual","igdbIds":[1,0]}`),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := newSnapshotService(t, nil, stubCatalog{}, []account.LayoutBlock{b}).Snapshot(); err == nil {
				t.Fatal("snapshot succeeded with an unusable layout block")
			}
		})
	}
}

func TestSnapshotLayoutGamesIgnoresUnknownBlockWhateverItsConfig(t *testing.T) {
	blocks := []account.LayoutBlock{{ID: "b1", Type: "hologram", Width: "full"}, block("b2", "hologram", `not json at all`)}
	snap, err := newSnapshotService(t, nil, stubCatalog{}, blocks).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.LayoutGames) != 0 {
		t.Fatalf("layoutGames = %+v", snap.LayoutGames)
	}
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	lib, log, cat := fixedLibrary{}, emptyLog{}, stubCatalog{}
	showcase := func() []string { return nil }
	cases := map[string]func() (*Service, error){
		"library":  func() (*Service, error) { return NewService(nil, log, cat, showcase, noLayout) },
		"log":      func() (*Service, error) { return NewService(lib, nil, cat, showcase, noLayout) },
		"catalog":  func() (*Service, error) { return NewService(lib, log, nil, showcase, noLayout) },
		"showcase": func() (*Service, error) { return NewService(lib, log, cat, nil, noLayout) },
		"layout":   func() (*Service, error) { return NewService(lib, log, cat, showcase, nil) },
	}
	for name, build := range cases {
		if s, err := build(); err == nil || s != nil {
			t.Errorf("%s: NewService = %v, %v, want an error", name, s, err)
		}
	}
}

func TestIGDBIDs(t *testing.T) {
	cat := stubCatalog{games: map[string]catalog.Game{
		"w3": catalogGame("w3", "1942", "Witcher 3"),
		"x":  catalogGame("x", "", "No IGDB"),
	}}
	s := newSnapshotService(t, nil, cat, nil)

	got, err := s.IGDBIDs(t.Context(), []string{"w3", "x", "missing", ""})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]string{"w3": "1942"}) {
		t.Fatalf("ids = %v", got)
	}

	empty, err := s.IGDBIDs(t.Context(), nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty input = %#v, %v", empty, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.IGDBIDs(ctx, []string{"w3"}); err == nil {
		t.Fatal("cancelled context was ignored")
	}
}

func TestPlayedGenresSkipsArchivedGames(t *testing.T) {
	cat := stubCatalog{games: map[string]catalog.Game{
		"a": catalogGame("a", "1", "A", "RPG"),
		"b": catalogGame("b", "2", "B", "Shooter"),
	}}
	games := []library.Game{
		{ID: "ga", CanonicalGameID: "a", PlaytimeSeconds: 100},
		{ID: "gb", CanonicalGameID: "b", PlaytimeSeconds: 9000, Archived: true},
	}
	snap, err := newSnapshotService(t, games, cat, nil).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want := GenreBreakdown{Genres: []GenreShare{{"RPG", 1}}}
	if !reflect.DeepEqual(snap.Genres, want) {
		t.Fatalf("genres = %+v, want %+v (the removed game must not count)", snap.Genres, want)
	}
}

func TestPlayedGenresBatchesCatalogLookups(t *testing.T) {
	var calls [][]string
	cat := stubCatalog{
		calls: &calls,
		games: map[string]catalog.Game{
			"a": catalogGame("a", "1", "A", "RPG"),
			"b": catalogGame("b", "2", "B", "Shooter"),
			"c": catalogGame("c", "3", "C", "Puzzle"),
		},
		aliases: map[string]string{"c-alias": "c"},
	}
	games := []library.Game{
		{ID: "1", CanonicalGameID: "a", PlaytimeSeconds: 100},
		{ID: "2", CanonicalGameID: "a", PlaytimeSeconds: 100},
		{ID: "3", CanonicalGameID: "b", PlaytimeSeconds: 100},
		{ID: "4", CanonicalGameID: "c", PlaytimeSeconds: 100},
		{ID: "5", CanonicalGameID: "c-alias", PlaytimeSeconds: 100},
		{ID: "6", CanonicalGameID: "missing", PlaytimeSeconds: 100},
	}
	s := newSnapshotService(t, games, cat, nil)

	got := s.playedGenres(games)
	wantGenres := [][]string{{"RPG"}, {"RPG"}, {"Shooter"}, {"Puzzle"}, {"Puzzle"}, nil}
	for i, entry := range got {
		if !reflect.DeepEqual(entry.Genres, wantGenres[i]) {
			t.Errorf("game %d genres = %v, want %v", i+1, entry.Genres, wantGenres[i])
		}
	}
	if len(calls) != 3 || len(calls[0]) != 5 {
		t.Fatalf("catalog calls = %v, want one batch of 5 distinct ids, then one lookup each for the alias and the missing id", calls)
	}

	calls = nil
	clean := []library.Game{{ID: "1", CanonicalGameID: "a", PlaytimeSeconds: 1}, {ID: "2", CanonicalGameID: "b", PlaytimeSeconds: 1}}
	s.playedGenres(clean)
	if len(calls) != 1 {
		t.Fatalf("catalog calls = %v, want a single batch when every id resolves", calls)
	}
}
