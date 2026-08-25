package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkUninstalledKeepsRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Kept")

	game, err := s.RegisterInstalled(InstalledGame{
		Title:       "Kept",
		Executable:  exe,
		InstallDir:  dir,
		Version:     "1.2",
		Owned:       true,
		InstallType: "portable",
		Uninstall:   Uninstall{Command: "unins000.exe"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatalf("mark uninstalled: %v", err)
	}

	if got := s.GetInstalledGames(); len(got) != 0 {
		t.Fatalf("installed = %+v", got)
	}
	all := s.GetGames()
	if len(all) != 1 || all[0].ID != game.ID {
		t.Fatalf("library = %+v", all)
	}
	kept := all[0]
	switch {
	case !kept.Uninstalled:
		t.Fatal("record is not marked as uninstalled")
	case kept.SizeBytes != 0 || kept.SizeUnknown:
		t.Fatalf("size = %d, unknown = %v", kept.SizeBytes, kept.SizeUnknown)
	case kept.Version != "":
		t.Fatalf("version = %q", kept.Version)
	case kept.Owned || kept.InstallType != "" || !kept.Uninstall.Empty():
		t.Fatalf("install traits kept: %+v", kept)
	case kept.Executable != exe || kept.InstallDir != dir:
		t.Fatalf("paths lost: %+v", kept)
	}

	reloaded := mustServiceAt(t, path).GetGames()
	if len(reloaded) != 1 || !reloaded[0].Uninstalled {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}

func TestMarkUninstalledUnknownGame(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if err := s.MarkUninstalled("nope"); !errors.Is(err, errNotFound) {
		t.Fatalf("mark uninstalled: %v", err)
	}
}

func TestMarkUninstalledKeepsPlaytime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Played")
	game, err := s.RegisterInstalled(InstalledGame{Title: "Played", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.games[0].PlaytimeSeconds = 7200
	s.mu.Unlock()

	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}
	all := s.GetGames()
	if len(all) != 1 || all[0].PlaytimeSeconds != 7200 {
		t.Fatalf("library = %+v", all)
	}
}

func TestPlayGameRefusesUninstalled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Gone")
	game, err := s.RegisterInstalled(InstalledGame{Title: "Gone", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("uninstalled game must not launch")
	}
}

func TestRegisterInstalledRevivesUninstalledRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	first, exe := discoveredDir(t, "Again")
	game, err := s.RegisterInstalled(InstalledGame{
		Title:           "Again",
		Executable:      exe,
		InstallDir:      first,
		CanonicalGameID: "canon-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}

	second, secondExe := discoveredDir(t, "AgainReinstalled")
	revived, err := s.RegisterInstalled(InstalledGame{
		Title:           "Again",
		Executable:      secondExe,
		InstallDir:      second,
		Version:         "2.0",
		CanonicalGameID: "canon-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if revived.ID != game.ID {
		t.Fatalf("second record created: %s != %s", revived.ID, game.ID)
	}
	installed := s.GetInstalledGames()
	if len(installed) != 1 {
		t.Fatalf("installed = %+v", installed)
	}
	switch {
	case installed[0].Uninstalled:
		t.Fatal("revived record still marked as uninstalled")
	case installed[0].InstallDir != second || installed[0].Executable != secondExe:
		t.Fatalf("paths not updated: %+v", installed[0])
	case installed[0].Version != "2.0":
		t.Fatalf("version = %q", installed[0].Version)
	}
}

func TestAddGameRevivesUninstalledRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Manual")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}

	again, err := s.AddGame(exe, "")
	if err != nil {
		t.Fatalf("add after uninstall: %v", err)
	}
	if again.ID != game.ID {
		t.Fatalf("second record created: %s != %s", again.ID, game.ID)
	}
	if again.Title != "Manual" {
		t.Fatalf("title = %q", again.Title)
	}
	installed := s.GetInstalledGames()
	if len(installed) != 1 || installed[0].Uninstalled || installed[0].SizeBytes == 0 {
		t.Fatalf("installed = %+v", installed)
	}
}

func TestApplyDiscoveredRevivesUninstalledRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Rescanned")
	game, err := s.RegisterInstalled(InstalledGame{Title: "Rescanned", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}

	size, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	found, outcome, err := s.ApplyDiscovered(Discovered{
		GameID:     game.ID,
		Title:      "Rescanned",
		Executable: exe,
		InstallDir: dir,
		SizeBytes:  size.Size(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeUpdated {
		t.Fatalf("outcome = %s", outcome)
	}
	if found.Uninstalled {
		t.Fatal("rediscovered game stayed uninstalled")
	}
	if len(s.GetInstalledGames()) != 1 {
		t.Fatalf("installed = %+v", s.GetInstalledGames())
	}
}
