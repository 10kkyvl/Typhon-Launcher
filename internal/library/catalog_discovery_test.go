package library

import (
	"path/filepath"
	"testing"
)

func TestApplyDiscoveredRevivesCatalogGameByCanonicalID(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	catalogOnly, err := s.AddCatalogGame("canon-portal", "Portal", "")
	if err != nil {
		t.Fatal(err)
	}

	found, outcome, err := s.ApplyDiscovered(Discovered{
		Title:           "Portal",
		Executable:      exe,
		InstallDir:      dir,
		SizeBytes:       4,
		CanonicalGameID: "canon-portal",
	})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	if outcome != OutcomeUpdated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeUpdated)
	}
	if found.ID != catalogOnly.ID {
		t.Fatalf("second record created: %s != %s", found.ID, catalogOnly.ID)
	}
	if found.Uninstalled {
		t.Fatal("record must no longer be uninstalled")
	}
	if found.InstallDir != dir || found.Executable != exe {
		t.Fatalf("found = %+v, want install dir %q and executable %q", found, dir, exe)
	}
	if games := s.GetGames(); len(games) != 1 {
		t.Fatalf("games = %+v, want exactly one", games)
	}
}

func TestApplyDiscoveredDoesNotMergeDifferentCanonicalID(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	if _, err := s.AddCatalogGame("canon-portal", "Portal", ""); err != nil {
		t.Fatal(err)
	}

	_, outcome, err := s.ApplyDiscovered(Discovered{
		Title:           "Something Else",
		Executable:      exe,
		InstallDir:      dir,
		SizeBytes:       4,
		CanonicalGameID: "canon-other",
	})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeCreated)
	}
	if games := s.GetGames(); len(games) != 2 {
		t.Fatalf("games = %+v, want two separate records", games)
	}
}

func TestApplyDiscoveredPathMatchOutranksCanonicalMatch(t *testing.T) {
	root := t.TempDir()
	realDir, realExe := gameDir(t, root, "RealGame")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	catalogOnly, err := s.AddCatalogGame("canon-shared", "Catalog Placeholder", "")
	if err != nil {
		t.Fatal(err)
	}
	realGame, outcome, err := s.ApplyDiscovered(Discovered{Title: "Real Game", Executable: realExe, InstallDir: realDir, SizeBytes: 4})
	if err != nil {
		t.Fatalf("apply discovered (create real): %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeCreated)
	}

	updated, outcome, err := s.ApplyDiscovered(Discovered{
		Title:           "Real Game",
		Executable:      realExe,
		InstallDir:      realDir,
		SizeBytes:       8,
		CanonicalGameID: "canon-shared",
	})
	if err != nil {
		t.Fatalf("apply discovered (rediscover): %v", err)
	}
	if outcome != OutcomeUpdated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeUpdated)
	}
	if updated.ID != realGame.ID {
		t.Fatalf("path match lost: matched %s, want the real record %s", updated.ID, realGame.ID)
	}
	if updated.ID == catalogOnly.ID {
		t.Fatal("path match must outrank the canonical-id match on the catalog-only record")
	}

	all := s.GetGames()
	if len(all) != 2 {
		t.Fatalf("games = %+v, want two records (real + untouched catalog-only)", all)
	}
	for _, g := range all {
		if g.ID == catalogOnly.ID && !g.Uninstalled {
			t.Fatalf("catalog-only record must stay untouched: %+v", g)
		}
	}
}

func TestApplyDiscoveredEmptyCanonicalIDDoesNotMergeWithEmptyCanonicalRecord(t *testing.T) {
	root := t.TempDir()
	oldDir, oldExe := gameDir(t, root, "NoCanonical")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	installed, err := s.RegisterInstalled(InstalledGame{Title: "No Canonical", Executable: oldExe, InstallDir: oldDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(installed.ID); err != nil {
		t.Fatal(err)
	}

	newDir, newExe := gameDir(t, root, "Unrelated")
	_, outcome, err := s.ApplyDiscovered(Discovered{Title: "Unrelated Game", Executable: newExe, InstallDir: newDir, SizeBytes: 4})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeCreated)
	}
	if games := s.GetGames(); len(games) != 2 {
		t.Fatalf("games = %+v, want two separate records", games)
	}
}
