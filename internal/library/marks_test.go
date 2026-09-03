package library

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func addGames(t *testing.T, s *Service, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		g, err := s.AddGame(tempGameExe(t), fmt.Sprintf("Game %d", i))
		if err != nil {
			t.Fatalf("add game: %v", err)
		}
		ids = append(ids, g.ID)
	}
	return ids
}

func TestSetFavoritePersistsAndCaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	ids := addGames(t, s, MaxFavorites+1)

	for _, id := range ids[:MaxFavorites] {
		g, err := s.SetFavorite(id, true)
		if err != nil {
			t.Fatalf("favorite %s: %v", id, err)
		}
		if !g.Favorite {
			t.Fatalf("returned game not favorite")
		}
	}
	if _, err := s.SetFavorite(ids[MaxFavorites], true); !errors.Is(err, ErrTooManyFavorites) {
		t.Fatalf("err = %v, want ErrTooManyFavorites", err)
	}
	if _, err := s.SetFavorite(ids[0], true); err != nil {
		t.Fatalf("re-favoriting an existing favorite must not count: %v", err)
	}
	if _, err := s.SetFavorite(ids[0], false); err != nil {
		t.Fatalf("unfavorite: %v", err)
	}
	if _, err := s.SetFavorite(ids[MaxFavorites], true); err != nil {
		t.Fatalf("slot freed, favorite must succeed: %v", err)
	}

	reloaded := mustServiceAt(t, path)
	count := 0
	for _, g := range reloaded.GetGames() {
		if g.Favorite {
			count++
		}
	}
	if count != MaxFavorites {
		t.Fatalf("favorites after reload = %d, want %d", count, MaxFavorites)
	}
}

func TestSetCompletedStampsTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	id := addGames(t, s, 1)[0]

	g, err := s.SetCompleted(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Completed || g.CompletedAt == nil || !g.CompletedAt.Equal(now) {
		t.Fatalf("game = %+v, want completed at %s", g, now)
	}
	g, err = s.SetCompleted(id, false)
	if err != nil {
		t.Fatal(err)
	}
	if g.Completed || g.CompletedAt != nil {
		t.Fatalf("game = %+v, want cleared", g)
	}
	if _, err := s.SetCompleted("missing", true); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}
