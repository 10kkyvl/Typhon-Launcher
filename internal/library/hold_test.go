package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/emulators"
	"typhon/internal/uierr"
)

// wholeRemoval runs a whole removal of the game at root (reserve, delete the
// folder, release) once, the first time it is called, and says how it went.
func wholeRemoval(t *testing.T, s *Service, id, root string) (func(), *string) {
	t.Helper()
	var once sync.Once
	outcome := new(string)
	return func() {
		once.Do(func() {
			release, err := s.BeginRemoval(id, root)
			if err != nil {
				*outcome = "refused: " + uierr.Code(err)
				return
			}
			defer release()
			if err := os.RemoveAll(root); err != nil {
				t.Error(err)
			}
			*outcome = "ran whole"
		})
	}, outcome
}

// duringMeasure runs remove when an adding path measures the folder: after it
// looked at its file, before it saves the card.
func duringMeasure(t *testing.T, remove func()) {
	t.Helper()
	old := measureInstall
	t.Cleanup(func() { measureInstall = old })
	measureInstall = func(context.Context, string, string) (int64, bool) {
		remove()
		return 1, false
	}
}

// duringImageRead runs remove when the image is read after it was checked.
func duringImageRead(t *testing.T, remove func()) {
	t.Helper()
	calls := 0
	stubImageStat(t, func(path string) (fs.FileInfo, error) {
		info, err := os.Lstat(path)
		if calls++; calls == 2 {
			remove()
		}
		return info, err
	})
}

func TestNoCardIsSavedForFilesDeletedWhileItWasAdded(t *testing.T) {
	add := map[string]func(t *testing.T, s *Service, root string, remove func()) (Game, error){
		"AddGame": func(t *testing.T, s *Service, root string, remove func()) (Game, error) {
			exe := writeFile(t, filepath.Join(root, "Bonus", "Game2.exe"), "stub")
			duringMeasure(t, remove)
			return s.AddGame(t.Context(), exe, "Bonus")
		},
		"RegisterInstalled": func(t *testing.T, s *Service, root string, remove func()) (Game, error) {
			exe := writeFile(t, filepath.Join(root, "Bonus", "Game2.exe"), "stub")
			duringMeasure(t, remove)
			return s.RegisterInstalled(t.Context(), InstalledGame{Title: "Bonus", Executable: exe, InstallDir: filepath.Dir(exe)})
		},
		"AddEmulatedGame": func(t *testing.T, s *Service, root string, remove func()) (Game, error) {
			iso := writeFile(t, filepath.Join(root, "ps2", "game.iso"), "12345678")
			duringImageRead(t, remove)
			return s.AddEmulatedGame(t.Context(), iso, emulators.PS2, "Bonus")
		},
		"RegisterEmulated": func(t *testing.T, s *Service, root string, remove func()) (Game, error) {
			iso := writeFile(t, filepath.Join(root, "ps2", "game.iso"), "12345678")
			duringImageRead(t, remove)
			return s.RegisterEmulated(t.Context(), InstalledImage{Title: "Bonus", Platform: emulators.PS2, Image: iso, InstallDir: filepath.Dir(iso), Owned: true})
		},
		"SetExecutable of an image": func(t *testing.T, s *Service, root string, remove func()) (Game, error) {
			card := emulatedGame(t, s, "elsewhere.iso")
			iso := writeFile(t, filepath.Join(root, "ps2", "game.iso"), "12345678")
			duringImageRead(t, remove)
			return s.SetExecutable(card.ID, iso)
		},
	}
	for name, adding := range add {
		t.Run(name, func(t *testing.T) {
			s := newLibrary(t)
			root := filepath.Join(t.TempDir(), "RepackX")
			a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
			remove, outcome := wholeRemoval(t, s, a.ID, root)

			g, err := adding(t, s, root, remove)

			if *outcome == "" {
				t.Fatal("the removal never ran: the hook missed the adding path")
			}
			if err != nil {
				return
			}
			if _, statErr := os.Stat(g.Executable); statErr != nil {
				t.Fatalf("card %q saved for a file deleted with another game (removal %s): %v", g.Title, *outcome, statErr)
			}
		})
	}
}

// A removal holds its card from its decision until it settles the card: every
// path that gives a card files again refuses that card meanwhile, or the
// removal would mark uninstalled or remove the card the player has just added.
func TestNoPathBindsFilesToACardARemovalHolds(t *testing.T) {
	native := func(t *testing.T, s *Service) Game {
		t.Helper()
		card := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
		if err := s.MarkUninstalled(card.ID); err != nil {
			t.Fatal(err)
		}
		return card
	}
	installed := func(t *testing.T, s *Service) Game {
		t.Helper()
		return addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
	}
	image := func(uninstalled bool) func(t *testing.T, s *Service) Game {
		return func(t *testing.T, s *Service) Game {
			t.Helper()
			card := emulatedGame(t, s, "game.iso")
			if uninstalled {
				if err := s.MarkUninstalled(card.ID); err != nil {
					t.Fatal(err)
				}
			}
			return card
		}
	}
	cases := []struct {
		name string
		card func(t *testing.T, s *Service) Game
		bind func(t *testing.T, s *Service, card Game) error
	}{
		{"AddGame", native, func(t *testing.T, s *Service, card Game) error {
			_, err := s.AddGame(t.Context(), card.Executable, card.Title)
			return err
		}},
		{"RegisterInstalled", native, func(t *testing.T, s *Service, card Game) error {
			_, err := s.RegisterInstalled(t.Context(), InstalledGame{Title: card.Title, Executable: card.Executable, InstallDir: card.InstallDir})
			return err
		}},
		{"ApplyDiscovered", native, func(t *testing.T, s *Service, card Game) error {
			_, _, err := s.ApplyDiscovered(Discovered{GameID: card.ID, Title: card.Title, Executable: card.Executable, InstallDir: card.InstallDir})
			return err
		}},
		{"AddEmulatedGame", image(true), func(t *testing.T, s *Service, card Game) error {
			_, err := s.AddEmulatedGame(t.Context(), card.Executable, emulators.PS2, card.Title)
			return err
		}},
		{"RegisterEmulated", image(true), func(t *testing.T, s *Service, card Game) error {
			_, err := s.RegisterEmulated(t.Context(), InstalledImage{Title: card.Title, Platform: emulators.PS2, Image: card.Executable, InstallDir: card.InstallDir})
			return err
		}},
		{"ApplyInstalledUpdate into another folder", installed, func(t *testing.T, s *Service, card Game) error {
			next := filepath.Join(t.TempDir(), "Game")
			_, err := s.ApplyInstalledUpdate(t.Context(), InstalledUpdate{ID: card.ID, InstallDir: next, Executable: writeFile(t, filepath.Join(next, "game.exe"), "stub")})
			return err
		}},
		{"SetExecutable of an image", image(false), func(t *testing.T, s *Service, card Game) error {
			_, err := s.SetExecutable(card.ID, writeFile(t, filepath.Join(t.TempDir(), "moved.iso"), "1234567"))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			card := tc.card(t, s)
			before, err := s.Find(card.ID)
			if err != nil {
				t.Fatal(err)
			}
			release, err := s.BeginRemoval(card.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			bindErr := tc.bind(t, s, before)
			release()

			requireCode(t, tc.name+" during the removal of its card", bindErr, "library.removing")
			after, err := s.Find(card.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Uninstalled != before.Uninstalled || after.Executable != before.Executable || after.InstallDir != before.InstallDir {
				t.Fatalf("card during its removal = %+v, want it as it was: %+v", after, before)
			}
		})
	}
}

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

	_, err := s.BeginRemoval(a.ID, a.InstallDir)
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

	release, err := s.BeginRemoval(a.ID, "")
	if err != nil {
		t.Fatalf("a removal that keeps the files = %v", err)
	}
	release()
}

func TestNoCardIsAddedInsideAFolderBeingDeleted(t *testing.T) {
	s := newLibrary(t)
	root := filepath.Join(t.TempDir(), "RepackX")
	a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
	release, err := s.BeginRemoval(a.ID, a.InstallDir)
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

	release, err := s.BeginRemoval(game.ID, game.InstallDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.BeginMove(game.ID)
	requireCode(t, "BeginMove during a removal", err, "library.removing")
	_, err = s.BeginRelocate(game.ID)
	requireCode(t, "BeginRelocate during a removal", err, "library.removing")
	_, err = s.BeginRemoval(game.ID, "")
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
	_, err = s.BeginRemoval(game.ID, "")
	requireCode(t, "BeginRemoval during a move", err, "library.relocating")
	requireCode(t, "RemoveGame during a move", s.RemoveGame(game.ID), "library.moving")
	requireCode(t, "MarkUninstalled during a move", s.MarkUninstalled(game.ID), "library.moving")
	release()
	release()

	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatalf("MarkUninstalled after the move = %v", err)
	}
}

// A footprint whose path cannot be resolved counts as overlapping: the check
// guards a deletion, so not knowing is a refusal, never a pass.
func TestOverlapFailsClosedOnAPathItCannotResolve(t *testing.T) {
	root := t.TempDir()
	games := filepath.Join(root, "Games")
	cases := []struct {
		name  string
		roots []string
		dir   string
		want  bool
	}{
		{"same folder", []string{filepath.Join(games, "Foo")}, filepath.Join(games, "Foo"), true},
		{"inside", []string{filepath.Join(games, "Foo", "Bonus")}, filepath.Join(games, "Foo"), true},
		{"around", []string{games}, filepath.Join(games, "Foo"), true},
		{"sibling sharing a prefix", []string{filepath.Join(games, "FooBar")}, filepath.Join(games, "Foo"), false},
		{"sibling the folder is a prefix of", []string{filepath.Join(games, "Foo")}, filepath.Join(games, "FooBar"), false},
		{"unresolvable footprint", []string{"   "}, filepath.Join(games, "Foo"), true},
		{"unresolvable folder", []string{filepath.Join(games, "Foo")}, "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlaps(tc.roots, tc.dir); got != tc.want {
				t.Fatalf("overlaps(%q, %q) = %v, want %v", tc.roots, tc.dir, got, tc.want)
			}
		})
	}
}
