package library

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func addAt(t *testing.T, s *Service, exe, title string) Game {
	t.Helper()
	writeFile(t, exe, "stub")
	game, err := s.AddGame(t.Context(), exe, title)
	if err != nil {
		t.Fatalf("add %s: %v", title, err)
	}
	return game
}

func requireCode(t *testing.T, what string, err error, want string) {
	t.Helper()
	if got := uierr.Code(err); got != want {
		t.Fatalf("%s = %q (%v), want %s", what, got, err, want)
	}
}

func TestRemovalWithFilesNamesTheGamesSharingTheFolder(t *testing.T) {
	s := newLibrary(t)
	root := filepath.Join(t.TempDir(), "RepackX")
	a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
	b := addAt(t, s, filepath.Join(root, "Bonus", "Game2.exe"), "Bonus")

	_, err := s.BeginRemoval(a.ID, true)
	requireCode(t, "BeginRemoval with files", err, "library.folder_shared")
	neighbours, err := s.FolderNeighbours(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(neighbours) != 1 || neighbours[0].ID != b.ID || neighbours[0].Title != "Bonus" {
		t.Fatalf("FolderNeighbours = %+v, want the card of Bonus", neighbours)
	}
	_, err = s.BeginMove(a.ID)
	requireCode(t, "BeginMove", err, "library.folder_shared")

	release, err := s.BeginRemoval(a.ID, false)
	if err != nil {
		t.Fatalf("a removal that keeps the files = %v", err)
	}
	release()
}

func TestNoCardIsAddedInsideAFolderBeingDeleted(t *testing.T) {
	s := newLibrary(t)
	root := filepath.Join(t.TempDir(), "RepackX")
	a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
	release, err := s.BeginRemoval(a.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "Bonus", "Game2.exe")
	writeFile(t, inside, "stub")

	_, err = s.AddGame(t.Context(), inside, "Bonus")
	requireCode(t, "AddGame inside a folder being deleted", err, "library.folder_busy")
	_, err = s.RegisterInstalled(t.Context(), InstalledGame{Title: "Bonus", Executable: inside, InstallDir: filepath.Dir(inside)})
	requireCode(t, "RegisterInstalled inside a folder being deleted", err, "library.folder_busy")
	if games := s.GetGames(); len(games) != 1 {
		t.Fatalf("cards = %+v, want only the game being removed", games)
	}
	reloaded := mustServiceAt(t, s.path)
	if games := reloaded.GetGames(); len(games) != 1 {
		t.Fatalf("saved cards = %+v, want only the game being removed", games)
	}

	release()
	if _, err := s.AddGame(t.Context(), inside, "Bonus"); err != nil {
		t.Fatalf("AddGame after the removal ended = %v", err)
	}
}

func TestUninstalledCardInsideDoesNotCountUntilItComesBack(t *testing.T) {
	s := newLibrary(t)
	root := filepath.Join(t.TempDir(), "RepackX")
	a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
	inside := filepath.Join(root, "Bonus", "Game2.exe")
	b := addAt(t, s, inside, "Bonus")
	if err := s.MarkUninstalled(b.ID); err != nil {
		t.Fatal(err)
	}
	release, err := s.BeginMove(a.ID)
	if err != nil {
		t.Fatalf("BeginMove with an uninstalled card inside = %v", err)
	}
	defer release()

	_, err = s.AddGame(t.Context(), inside, "Bonus")
	requireCode(t, "bringing the uninstalled card back into a folder being moved", err, "library.folder_busy")
	game, err := s.Find(b.ID)
	if err != nil || !game.Uninstalled {
		t.Fatalf("card = %+v, %v; want it still uninstalled", game, err)
	}
}

func TestRemovalAndMoveOfAGameExcludeEachOther(t *testing.T) {
	s := newLibrary(t)
	exe := filepath.Join(t.TempDir(), "Game", "game.exe")
	game := addAt(t, s, exe, "Game")

	release, err := s.BeginRemoval(game.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.BeginMove(game.ID)
	requireCode(t, "BeginMove during a removal", err, "library.removing")
	_, err = s.BeginRelocate(game.ID)
	requireCode(t, "BeginRelocate during a removal", err, "library.removing")
	_, err = s.BeginRemoval(game.ID, false)
	requireCode(t, "a second removal", err, "library.removing")
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	requireCode(t, "PlayGame during a removal", s.PlayGame(game.ID), "library.removing")
	release()

	writeFile(t, exe, "stub")
	release, err = s.BeginMove(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.BeginRemoval(game.ID, false)
	requireCode(t, "BeginRemoval during a move", err, "library.relocating")
	requireCode(t, "RemoveGame during a move", s.RemoveGame(game.ID), "library.moving")
	requireCode(t, "MarkUninstalled during a move", s.MarkUninstalled(game.ID), "library.moving")
	release()
	release()

	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatalf("MarkUninstalled after the move = %v", err)
	}
}
