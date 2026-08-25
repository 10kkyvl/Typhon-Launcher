package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAddCatalogGameCreatesRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	game, err := s.AddCatalogGame("canon-1", "Some Game", "https://example/cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !game.Uninstalled {
		t.Fatal("catalog-only record must be marked uninstalled")
	}
	if game.InstallDir != "" || game.Executable != "" {
		t.Fatalf("game = %+v, want empty install dir and executable", game)
	}
	if game.CanonicalGameID != "canon-1" || game.Title != "Some Game" {
		t.Fatalf("game = %+v", game)
	}

	all := s.GetGames()
	if len(all) != 1 || all[0].ID != game.ID {
		t.Fatalf("games = %+v", all)
	}
}

func TestAddCatalogGameExcludedFromInstalled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	if _, err := s.AddCatalogGame("canon-1", "Some Game", ""); err != nil {
		t.Fatal(err)
	}
	if got := s.GetInstalledGames(); len(got) != 0 {
		t.Fatalf("installed = %+v, want none", got)
	}
}

func TestAddCatalogGameIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	first, err := s.AddCatalogGame("canon-1", "Some Game", "cover-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddCatalogGame("canon-1", "Renamed", "cover-b")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("second call created a new record: %s != %s", second.ID, first.ID)
	}
	if got := s.GetGames(); len(got) != 1 {
		t.Fatalf("games = %+v, want exactly one", got)
	}
}

func TestAddCatalogGameIdempotentWhenInstalled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Installed")

	installed, err := s.RegisterInstalled(InstalledGame{
		Title:           "Installed",
		Executable:      exe,
		InstallDir:      dir,
		CanonicalGameID: "canon-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	again, err := s.AddCatalogGame("canon-1", "Installed", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != installed.ID {
		t.Fatalf("second record created: %s != %s", again.ID, installed.ID)
	}
	if again.InstallDir != dir || again.Uninstalled {
		t.Fatalf("existing installed record was overwritten: %+v", again)
	}
	if got := s.GetGames(); len(got) != 1 {
		t.Fatalf("games = %+v, want exactly one", got)
	}
}

func TestAddCatalogGameRejectsEmptyCanonicalID(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if _, err := s.AddCatalogGame("  ", "Some Game", ""); !errors.Is(err, errEmptyCanonicalGameID) {
		t.Fatalf("err = %v, want %v", err, errEmptyCanonicalGameID)
	}
	if got := s.GetGames(); len(got) != 0 {
		t.Fatalf("games = %+v, want none", got)
	}
}

func TestAddCatalogGameRejectsEmptyTitle(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if _, err := s.AddCatalogGame("canon-1", "  ", ""); !errors.Is(err, errEmptyCatalogTitle) {
		t.Fatalf("err = %v, want %v", err, errEmptyCatalogTitle)
	}
	if got := s.GetGames(); len(got) != 0 {
		t.Fatalf("games = %+v, want none", got)
	}
}

func TestRemoveGameRemovesCatalogOnlyRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetGames(); len(got) != 0 {
		t.Fatalf("games = %+v, want none after removal", got)
	}
}

func TestPlayCatalogOnlyGameRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("expected error for catalog-only record")
	}
}

func TestAddCatalogGameWriteFailureRollsBackMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddCatalogGame("canon-1", "Some Game", ""); err == nil {
		t.Fatal("expected persist failure")
	}
	if got := s.GetGames(); len(got) != 0 {
		t.Fatalf("games = %+v, want none after rollback", got)
	}
}

func TestRegisterInstalledRevivesCatalogOnlyRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	catalogOnly, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}

	dir, exe := discoveredDir(t, "SomeGame")
	installed, err := s.RegisterInstalled(InstalledGame{
		Title:           "Some Game",
		Executable:      exe,
		InstallDir:      dir,
		CanonicalGameID: "canon-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if installed.ID != catalogOnly.ID {
		t.Fatalf("second record created: %s != %s", installed.ID, catalogOnly.ID)
	}
	if installed.Uninstalled || installed.InstallDir != dir {
		t.Fatalf("record not revived: %+v", installed)
	}
	if got := s.GetGames(); len(got) != 1 {
		t.Fatalf("games = %+v, want exactly one", got)
	}
}
