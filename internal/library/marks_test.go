package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
	if got, want := ErrTooManyFavorites.Error(), "favorites limit reached"; got != want {
		t.Fatalf("ErrTooManyFavorites.Error() = %q, want %q (matched by substring in frontend/src/lib/game/markMessages.ts)", got, want)
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

func TestSetFavoriteNoopDoesNotRewriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	ids := addGames(t, s, 1)
	id := ids[0]

	if _, err := s.SetFavorite(id, true); err != nil {
		t.Fatalf("favorite: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetFavorite(id, true); err != nil {
		t.Fatalf("re-favorite: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("file bytes changed on a no-op re-favorite")
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("mtime changed on a no-op re-favorite: before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

func TestSetStatusStampsAndValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	id := addGames(t, s, 1)[0]

	g, err := s.SetStatus(id, StatusCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != StatusCompleted || g.StatusAt == nil || !g.StatusAt.Equal(now) {
		t.Fatalf("game = %+v, want completed at %s", g, now)
	}
	before, _ := os.ReadFile(path)
	if _, err := s.SetStatus(id, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("no-op status change rewrote the file")
	}
	g, err = s.SetStatus(id, "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != "" || g.StatusAt != nil {
		t.Fatalf("game = %+v, want cleared", g)
	}
	if _, err := s.SetStatus(id, "won"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("err = %v, want ErrInvalidStatus", err)
	}
	if _, err := s.SetStatus("missing", StatusPlaying); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestSetFavoriteStampsStatusAt(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	id := addGames(t, s, 1)[0]
	g, err := s.SetFavorite(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if g.StatusAt == nil || !g.StatusAt.Equal(now) {
		t.Fatalf("favorite must stamp StatusAt, got %+v", g)
	}
}
