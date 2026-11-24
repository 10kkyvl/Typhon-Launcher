package library

import (
	"errors"
	"path/filepath"
	"testing"
)

// An update registered while its game moves would name the folder the move is
// about to delete, or move the card back to it: it is refused, the card stays
// as the move left it, and it is applied once the move is over.
func TestApplyInstalledUpdateRefusesWhileTheGameMoves(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reserve func(s *Service, card Game) (func(), error)
		dir     func(card Game) string
	}{
		{"live move, the card's folder", func(s *Service, card Game) (func(), error) {
			return s.BeginMove(card.ID, filepath.Join(t.TempDir(), "Game"))
		}, func(card Game) string { return card.InstallDir }},
		{"live move, no folder given", func(s *Service, card Game) (func(), error) {
			return s.BeginMove(card.ID)
		}, func(Game) string { return "" }},
		{"recovery of a move", func(s *Service, card Game) (func(), error) {
			return s.BeginRecovery(card.ID, card.InstallDir, filepath.Join(t.TempDir(), "Game"))
		}, func(card Game) string { return card.InstallDir }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			card := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
			release, err := tc.reserve(s, card)
			if err != nil {
				t.Fatal(err)
			}

			_, applyErr := s.ApplyInstalledUpdate(t.Context(), InstalledUpdate{ID: card.ID, InstallDir: tc.dir(card), Version: "2"})
			requireCode(t, "ApplyInstalledUpdate during the move", applyErr, "library.moving")
			after, err := s.Find(card.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Version != card.Version || after.InstallDir != card.InstallDir {
				t.Fatalf("card during the move = %+v, want it as it was", after)
			}

			release()
			updated, err := s.ApplyInstalledUpdate(t.Context(), InstalledUpdate{ID: card.ID, InstallDir: tc.dir(card), Version: "2"})
			if err != nil || updated.Version != "2" {
				t.Fatalf("ApplyInstalledUpdate after the move = %+v, %v; want version 2", updated, err)
			}
		})
	}
}

// An update registers the folder it wrote; a card that names another folder
// since (a move finished, an install or a scan rebound it) is not moved back
// to it.
func TestApplyInstalledUpdateRefusesAFolderTheCardNoLongerNames(t *testing.T) {
	s := newLibrary(t)
	card := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
	old := filepath.Join(t.TempDir(), "Game")
	exe := writeFile(t, filepath.Join(old, "game.exe"), "version 2")

	_, err := s.ApplyInstalledUpdate(t.Context(), InstalledUpdate{ID: card.ID, InstallDir: old, Executable: exe, Version: "2"})

	requireCode(t, "ApplyInstalledUpdate into a folder the card does not name", err, "library.installation_changed")
	if !errors.Is(err, ErrFolderChanged) {
		t.Fatalf("err = %v, want ErrFolderChanged", err)
	}
	after, err := s.Find(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.InstallDir != card.InstallDir || after.Executable != card.Executable || after.Version != card.Version {
		t.Fatalf("card = %+v, want it as it was: %+v", after, card)
	}
}

// An update or a repair writes the game's folder from its start to its end:
// it and a move of the game, live or recovered, exclude each other in the
// library's lock, whichever comes first, and a released hold lets the other in.
func TestUpdateAndMoveOfAGameExcludeEachOther(t *testing.T) {
	s := newLibrary(t)
	card := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
	target := filepath.Join(t.TempDir(), "Game")

	releaseUpdate, err := s.BeginUpdate(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginMove(card.ID, target); !errors.Is(err, ErrGameUpdating) {
		t.Fatalf("BeginMove during an update = %v, want ErrGameUpdating", err)
	}
	if _, err := s.BeginRecovery(card.ID, card.InstallDir, target); !errors.Is(err, ErrGameUpdating) {
		t.Fatalf("BeginRecovery during an update = %v, want ErrGameUpdating", err)
	}
	releaseUpdate()
	releaseUpdate()

	releaseMove, err := s.BeginMove(card.ID, target)
	if err != nil {
		t.Fatalf("BeginMove after the update = %v", err)
	}
	if _, err := s.BeginUpdate(card.ID); !errors.Is(err, ErrGameRelocating) {
		t.Fatalf("BeginUpdate during a move = %v, want ErrGameRelocating", err)
	}
	releaseMove()

	releaseRemoval, err := s.BeginRemoval(card.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginUpdate(card.ID); !errors.Is(err, ErrGameRemoving) {
		t.Fatalf("BeginUpdate during a removal = %v, want ErrGameRemoving", err)
	}
	releaseRemoval()

	release, err := s.BeginUpdate(card.ID)
	if err != nil {
		t.Fatalf("BeginUpdate with nothing else held = %v", err)
	}
	release()
	if _, err := s.BeginUpdate("missing"); err == nil {
		t.Fatal("BeginUpdate of a game not in the library granted")
	}
}

// The card's own folder, given or left empty, is what every update registers.
func TestApplyInstalledUpdateRegistersTheCardsOwnFolder(t *testing.T) {
	for _, tc := range []struct {
		name string
		dir  func(card Game) string
	}{
		{"given", func(card Game) string { return card.InstallDir }},
		{"empty", func(Game) string { return "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			card := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
			updated, err := s.ApplyInstalledUpdate(t.Context(), InstalledUpdate{ID: card.ID, InstallDir: tc.dir(card), Executable: card.Executable, Version: "2"})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Version != "2" || updated.InstallDir != card.InstallDir {
				t.Fatalf("updated = %+v, want version 2 at %s", updated, card.InstallDir)
			}
		})
	}
}
