package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGameByIGDB(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Cyberpunk 2077", ExternalIDs: ExternalIDs{IGDB: "1877"}},
		Game{Title: "Portal 2"},
	)
	withIGDB := games[0]

	cases := []struct {
		name   string
		id     string
		wantID string
		wantOK bool
	}{
		{"known id", "1877", withIGDB.ID, true},
		{"known id different case", "1877", withIGDB.ID, true},
		{"unknown id", "999999", "", false},
		{"empty id", "", "", false},
		{"case insensitive match", "1877", withIGDB.ID, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := s.GameByIGDB(tc.id)
			if ok != tc.wantOK {
				t.Fatalf("GameByIGDB(%q) ok = %v, want %v", tc.id, ok, tc.wantOK)
			}
			if ok && got.ID != tc.wantID {
				t.Fatalf("GameByIGDB(%q) id = %q, want %q", tc.id, got.ID, tc.wantID)
			}
		})
	}

	t.Run("mixed case id", func(t *testing.T) {
		s := newTestService(t)
		games := seed(t, s, Game{Title: "Half-Life 2", ExternalIDs: ExternalIDs{IGDB: "AbC123"}})
		got, ok := s.GameByIGDB("aBc123")
		if !ok || got.ID != games[0].ID {
			t.Fatalf("GameByIGDB case-insensitive lookup failed: got=%+v ok=%v", got, ok)
		}
	})

	t.Run("empty catalog does not panic", func(t *testing.T) {
		s := newTestService(t)
		got, ok := s.GameByIGDB("1877")
		if ok {
			t.Fatalf("expected no match on empty catalog, got %+v", got)
		}
	})
}

func TestEnsureByIGDBRejectsEmptyInput(t *testing.T) {
	cases := []struct {
		name   string
		igdbID string
		title  string
		want   error
	}{
		{"empty igdb id", "", "Cyberpunk 2077", errEmptyIGDBID},
		{"whitespace igdb id", "   ", "Cyberpunk 2077", errEmptyIGDBID},
		{"empty title", "1877", "", errEmptyCatalogTitle},
		{"whitespace title", "1877", "   ", errEmptyCatalogTitle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t)
			if _, err := s.EnsureByIGDB(tc.igdbID, tc.title); !errors.Is(err, tc.want) {
				t.Fatalf("EnsureByIGDB(%q, %q) err = %v, want %v", tc.igdbID, tc.title, err, tc.want)
			}
			if len(s.ListGames()) != 0 {
				t.Fatalf("games = %+v, want none created on rejected input", s.ListGames())
			}
		})
	}
}

func TestEnsureByIGDBCreatesOnFirstCall(t *testing.T) {
	s := newTestService(t)
	game, err := s.EnsureByIGDB("1877", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if game.ExternalIDs.IGDB != "1877" {
		t.Fatalf("igdb id = %q, want 1877", game.ExternalIDs.IGDB)
	}
	if !game.Provisional {
		t.Fatal("created game should be provisional")
	}
	if len(s.ListGames()) != 1 {
		t.Fatalf("games = %d, want 1", len(s.ListGames()))
	}

	got, ok := s.GameByIGDB("1877")
	if !ok || got.ID != game.ID {
		t.Fatalf("GameByIGDB after ensure = %+v, ok=%v, want %s", got, ok, game.ID)
	}
}

func TestEnsureByIGDBFindsExistingRegardlessOfTitleDrift(t *testing.T) {
	s := newTestService(t)
	first, err := s.EnsureByIGDB("1877", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	second, err := s.EnsureByIGDB("1877", "Cyberpunk 2077: Ultimate Edition")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("second ensure created a new game: %s != %s", second.ID, first.ID)
	}
	if second.Title != first.Title {
		t.Fatalf("existing game was renamed: %q != %q", second.Title, first.Title)
	}
	if len(s.ListGames()) != 1 {
		t.Fatalf("games = %d, want 1 (no duplicate by title drift)", len(s.ListGames()))
	}
}

func TestEnsureByIGDBWriteFailureRollsBackMemory(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	gamesPath := filepath.Join(dir, "catalog.json")
	if err := os.Remove(gamesPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(gamesPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := s.EnsureByIGDB("1877", "Cyberpunk 2077"); err == nil {
		t.Fatal("expected persist failure")
	}

	if len(s.ListGames()) != 0 {
		t.Fatalf("games = %+v, want none after rollback", s.ListGames())
	}
	if got, ok := s.GameByIGDB("1877"); ok {
		t.Fatalf("GameByIGDB found a partially added game: %+v", got)
	}
	if len(s.SearchGames("Cyberpunk", 5)) != 0 {
		t.Fatal("search should not find a rolled back game")
	}
}

func TestOpenByIGDBReturnsExistingGame(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s, Game{Title: "Cyberpunk 2077", ExternalIDs: ExternalIDs{IGDB: "1877"}})
	existing := games[0]

	got, err := s.OpenByIGDB("1877", "Cyberpunk 2077: Ultimate Edition")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got.ID != existing.ID {
		t.Fatalf("id = %q, want %q", got.ID, existing.ID)
	}
	if got.Title != existing.Title {
		t.Fatalf("title = %q, want %q (a drifted title must not rename the catalog entry)", got.Title, existing.Title)
	}
	if len(s.ListGames()) != 1 {
		t.Fatalf("games = %d, want 1", len(s.ListGames()))
	}
}

func TestOpenByIGDBCreatesWhenMissing(t *testing.T) {
	s := newTestService(t)
	got, err := s.OpenByIGDB("1877", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got.ID == "" {
		t.Fatal("created game has no id, the page would have nothing to navigate to")
	}
	if got.ExternalIDs.IGDB != "1877" {
		t.Fatalf("igdb id = %q, want 1877", got.ExternalIDs.IGDB)
	}

	again, err := s.OpenByIGDB("1877", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if again.ID != got.ID {
		t.Fatalf("second open created a duplicate: %s != %s", again.ID, got.ID)
	}
}

func TestOpenByIGDBRejectsEmptyInput(t *testing.T) {
	cases := []struct {
		name   string
		igdbID string
		title  string
		want   error
	}{
		{"empty igdb id", "", "Cyberpunk 2077", errEmptyIGDBID},
		{"empty title", "1877", "  ", errEmptyCatalogTitle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t)
			if _, err := s.OpenByIGDB(tc.igdbID, tc.title); !errors.Is(err, tc.want) {
				t.Fatalf("OpenByIGDB(%q, %q) err = %v, want %v", tc.igdbID, tc.title, err, tc.want)
			}
			if len(s.ListGames()) != 0 {
				t.Fatalf("games = %+v, want none created on rejected input", s.ListGames())
			}
		})
	}
}
