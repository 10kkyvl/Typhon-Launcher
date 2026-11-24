package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// A store sync and a move's repoint give a card a folder without an adding
// hold: the save itself refuses a card that would lie in a folder being
// deleted, and the memory goes back to what is on disk.
func TestNoCardIsSavedInsideAFolderBeingDeletedByAPathWithoutAnAddingHold(t *testing.T) {
	cases := []struct {
		name string
		bind func(t *testing.T, s *Service, inside string) error
	}{
		{"store sync", func(t *testing.T, s *Service, inside string) error {
			exe := writeFile(t, filepath.Join(inside, "Game2.exe"), "stub")
			_, err := s.ApplyExternal(t.Context(), []ExternalGame{{Provider: ProviderSteam, AppID: "42", Title: "Bonus", InstallDir: inside, Executable: exe}})
			return err
		}},
		{"move repoint", func(t *testing.T, s *Service, inside string) error {
			b := addAt(t, s, filepath.Join(t.TempDir(), "Bonus", "Game2.exe"), "Bonus")
			_, err := s.Relocate(b.ID, mustDir(t, inside))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			root := filepath.Join(t.TempDir(), "RepackX")
			a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
			inside := filepath.Join(root, "Bonus")
			release, err := s.BeginRemoval(a.ID, root)
			if err != nil {
				t.Fatal(err)
			}
			bindErr := tc.bind(t, s, inside)
			release()

			requireCode(t, tc.name+" into a folder being deleted", bindErr, "library.folder_busy")
			for _, g := range append(s.GetGames(), mustServiceAt(t, s.path).GetGames()...) {
				if g.ID != a.ID && overlaps(g.Footprint(), root) {
					t.Fatalf("card %+v lies in the folder being deleted", g)
				}
			}
		})
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
	_, err = s.BeginRecovery(game.ID, game.InstallDir, game.InstallDir+"-moved")
	requireCode(t, "BeginRecovery during a removal", err, "library.removing")
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

// A removal deletes, and a live move takes along, everything in the game's
// folder: a card added around that folder is refused as one added at or inside
// it, by the adding hold and by the save's backstop.
func TestRemovalAndMoveHoldsRefuseACardAroundAtOrInside(t *testing.T) {
	holds := []struct {
		name  string
		begin func(s *Service, a Game) (func(), error)
	}{
		{"removal", func(s *Service, a Game) (func(), error) { return s.BeginRemoval(a.ID, a.InstallDir) }},
		{"live move", func(s *Service, a Game) (func(), error) { return s.BeginMove(a.ID) }},
	}
	for _, h := range holds {
		for _, where := range []string{"around", "at", "inside"} {
			t.Run(h.name+"/"+where, func(t *testing.T) {
				s := newLibrary(t)
				root := filepath.Join(t.TempDir(), "Games", "RepackX")
				a := addAt(t, s, filepath.Join(root, "game.exe"), "RepackX")
				release, err := h.begin(s, a)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				var exe string
				switch where {
				case "around":
					exe = filepath.Join(filepath.Dir(root), "tool.exe")
				case "at":
					exe = filepath.Join(root, "tool.exe")
				default:
					exe = filepath.Join(root, "Bonus", "tool.exe")
				}
				_, err = s.AddGame(t.Context(), writeFile(t, exe, "stub"), "Tool")
				requireCode(t, "AddGame "+where, err, "library.folder_busy")
				other := addAt(t, s, filepath.Join(t.TempDir(), "Other", "o.exe"), "Other")
				_, err = s.Relocate(other.ID, filepath.Dir(exe))
				requireCode(t, "Relocate "+where+" (the save's backstop)", err, "library.folder_busy")
			})
		}
	}
}

// A live move holds the folders it writes (target, staging) as a recovery
// holds its copies: a card at or inside one refuses the move, and none can be
// added there until release; a card around them is how every library folder
// is reached and counts for neither.
func TestLiveMoveHoldsItsCopiesAgainstCardsAtOrInside(t *testing.T) {
	for _, where := range []string{"around", "at", "inside", "in the staging"} {
		t.Run(where, func(t *testing.T) {
			exeIn := func(target string) string {
				switch where {
				case "around":
					return filepath.Join(filepath.Dir(target), "tool.exe")
				case "at":
					return filepath.Join(target, "tool.exe")
				case "inside":
					return filepath.Join(target, "Bonus", "tool.exe")
				}
				return filepath.Join(target+".staging", "tool.exe")
			}
			s := newLibrary(t)
			game := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
			target := filepath.Join(t.TempDir(), "Games", "Game")

			release, err := s.BeginMove(game.ID, target, target+".staging")
			if err != nil {
				t.Fatal(err)
			}
			_, addErr := s.AddGame(t.Context(), writeFile(t, exeIn(target), "stub"), "Tool")
			release()
			if where == "around" {
				if addErr != nil {
					t.Fatalf("AddGame around the move's target = %v", addErr)
				}
				return
			}
			requireCode(t, "AddGame "+where+" the move's target", addErr, "library.folder_busy")

			if _, err := s.AddGame(t.Context(), exeIn(target), "Tool"); err != nil {
				t.Fatal(err)
			}
			_, err = s.BeginMove(game.ID, target, target+".staging")
			requireCode(t, "BeginMove with a card "+where+" its target", err, "library.folder_shared")
		})
	}
}

// A removal or a move of one game is refused while its folder is at, inside or
// around a folder another game's removal, move or recovery holds: deleting or
// moving it would take the other game's files being deleted, read or written
// along. A recovery holds its copy only against cards inside it, so a card
// around the copy exists; deleting that card's folder is what is refused.
func TestRemovalOrMoveRefusedAroundAFolderAnotherGameHolds(t *testing.T) {
	type rig struct {
		s      *Service
		parent string
		game   Game
	}
	holders := []struct {
		name string
		hold func(t *testing.T, r rig) (func(), error)
		// inHeld is a folder inside what the holder holds.
		inHeld func(r rig) string
	}{
		{"recovery into the folder", func(_ *testing.T, r rig) (func(), error) {
			target := filepath.Join(r.parent, "Game")
			return r.s.BeginRecovery(r.game.ID, r.game.InstallDir, target, target+".staging")
		}, func(r rig) string { return filepath.Join(r.parent, "Game", "Sub") }},
		{"removal of a folder", func(_ *testing.T, r rig) (func(), error) {
			return r.s.BeginRemoval(r.game.ID, r.game.InstallDir)
		}, func(r rig) string { return filepath.Join(r.game.InstallDir, "Sub") }},
		{"live move of a folder", func(_ *testing.T, r rig) (func(), error) {
			return r.s.BeginMove(r.game.ID)
		}, func(r rig) string { return filepath.Join(r.game.InstallDir, "Sub") }},
		{"live move into the folder", func(_ *testing.T, r rig) (func(), error) {
			target := filepath.Join(r.parent, "Game")
			return r.s.BeginMove(r.game.ID, target, target+".staging")
		}, func(r rig) string { return filepath.Join(r.parent, "Game", "Sub") }},
		{"live move into the folder, its staging", func(_ *testing.T, r rig) (func(), error) {
			target := filepath.Join(r.parent, "Game")
			return r.s.BeginMove(r.game.ID, target, target+".staging")
		}, func(r rig) string { return filepath.Join(r.parent, "Game.staging", "Sub") }},
	}
	for _, h := range holders {
		t.Run(h.name, func(t *testing.T) {
			s := newLibrary(t)
			parent := filepath.Join(t.TempDir(), "Games")
			r := rig{s: s, parent: parent, game: addAt(t, s, filepath.Join(t.TempDir(), "Games", "Game", "game.exe"), "Game")}
			var tool Game
			if strings.Contains(h.name, "into the folder") {
				tool = addAt(t, s, filepath.Join(parent, "tool.exe"), "Tool")
			}
			release, err := h.hold(t, r)
			if err != nil {
				t.Fatalf("holder = %v", err)
			}
			defer release()

			if tool.ID != "" {
				_, err = s.BeginRemoval(tool.ID, parent)
				requireCode(t, "removal with files of the folder around a recovering copy", err, "library.folder_busy")
				_, err = s.BeginMove(tool.ID)
				requireCode(t, "move of the folder around a recovering copy", err, "library.folder_busy")
				if !strings.Contains(err.Error(), "Game") {
					t.Fatalf("refusal %q does not name the game holding the folder", err)
				}
			}
			other := addAt(t, s, filepath.Join(t.TempDir(), "Other", "other.exe"), "Other")
			_, err = s.BeginRecovery(other.ID, other.InstallDir, h.inHeld(r), h.inHeld(r)+".staging")
			requireCode(t, "recovery of another game into a held folder", err, "library.folder_busy")

			sibling := addAt(t, s, filepath.Join(t.TempDir(), "Sibling", "sibling.exe"), "Sibling")
			rel, err := s.BeginRemoval(sibling.ID, sibling.InstallDir)
			if err != nil {
				t.Fatalf("removal of a folder clear of every hold = %v", err)
			}
			rel()
			rel, err = s.BeginRecovery(sibling.ID, sibling.InstallDir, filepath.Join(parent, "Sibling"))
			if err != nil {
				t.Fatalf("recovery into a sibling of a held folder = %v", err)
			}
			rel()
		})
	}
}

// Recovery of a move checks the old folder as a live move checks the game's
// folder, and the folders the copy is in (target, staging) only for a card at
// or inside them: a card whose folder holds them is how every library folder
// looks. A card being added is a passing refusal of its own.
func TestRecoveryReservationChecksTheCopiesOnlyForCardsInside(t *testing.T) {
	cases := []struct {
		name  string
		other func(source, target string) string
		want  string
	}{
		{"card around the target", func(_, target string) string { return filepath.Join(filepath.Dir(target), "tool.exe") }, ""},
		{"card inside the target", func(_, target string) string { return filepath.Join(target, "Bonus", "tool.exe") }, "library.folder_shared"},
		{"card in the staging folder", func(_, target string) string { return filepath.Join(target+".staging", "tool.exe") }, "library.folder_shared"},
		{"card around the old folder", func(source, _ string) string { return filepath.Join(filepath.Dir(source), "tool.exe") }, "library.folder_shared"},
		{"card inside the old folder", func(source, _ string) string { return filepath.Join(source, "Bonus", "tool.exe") }, "library.folder_shared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLibrary(t)
			game := addAt(t, s, filepath.Join(t.TempDir(), "Games", "Game", "game.exe"), "Game")
			target := filepath.Join(t.TempDir(), "Games", "Game")
			addAt(t, s, tc.other(game.InstallDir, target), "Tool")

			release, err := s.BeginRecovery(game.ID, game.InstallDir, target, target+".staging")
			if tc.want != "" {
				requireCode(t, "BeginRecovery", err, tc.want)
				return
			}
			if err != nil {
				t.Fatalf("BeginRecovery = %v", err)
			}
			defer release()
			if _, err := s.Relocate(game.ID, mustDir(t, target)); err != nil {
				t.Fatalf("repoint while the recovery holds the copy = %v", err)
			}
			_, err = s.AddGame(t.Context(), writeFile(t, filepath.Join(target, "Late", "late.exe"), "stub"), "Late")
			requireCode(t, "AddGame inside the copy during its recovery", err, "library.folder_busy")
			if _, err := s.AddGame(t.Context(), writeFile(t, filepath.Join(filepath.Dir(target), "tool2.exe"), "stub"), "Tool 2"); err != nil {
				t.Fatalf("AddGame of a folder that holds the copy during its recovery = %v", err)
			}
		})
	}
}

func TestRecoveryReservationWaitsForACardBeingAddedInside(t *testing.T) {
	s := newLibrary(t)
	game := addAt(t, s, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
	target := filepath.Join(t.TempDir(), "Game")
	var recoverErr error
	duringMeasure(t, func() {
		release, err := s.BeginRecovery(game.ID, game.InstallDir, target)
		if err == nil {
			release()
		}
		recoverErr = err
	})
	if _, err := s.AddGame(t.Context(), writeFile(t, filepath.Join(target, "Late", "late.exe"), "stub"), "Late"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, "BeginRecovery while a card is added inside the copy", recoverErr, "library.folder_busy")
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
