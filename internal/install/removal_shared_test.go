package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/emulators"
	"typhon/internal/library"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// sharedRig removes games through the real library: the folder checks and the
// reservations of a game live there, and a fake would only repeat the test.
type sharedRig struct {
	s   *Service
	lib *library.Service
}

func newSharedRig(t *testing.T) sharedRig {
	t.Helper()
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("new library: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	s := mustServiceAt(t, t.TempDir())
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	s.library = lib
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return sharedRig{s: s, lib: lib}
}

// ownedGame installs a launcher-owned portable game in dir.
func (r sharedRig) ownedGame(t *testing.T, dir, title string, uninstall library.Uninstall) library.Game {
	t.Helper()
	exe := filepath.Join(dir, title+".exe")
	mkFile(t, exe, 1024)
	mkFile(t, filepath.Join(dir, "data", "content.pak"), 2048)
	game, err := r.lib.RegisterInstalled(t.Context(), library.InstalledGame{
		Title: title, Executable: exe, InstallDir: dir,
		InstallType: string(TypePortable), Owned: true, Uninstall: uninstall,
	})
	if err != nil {
		t.Fatalf("register %s: %v", title, err)
	}
	return game
}

func (r sharedRig) addedGame(t *testing.T, exe, title string) library.Game {
	t.Helper()
	mkFile(t, exe, 512)
	game, err := r.lib.AddGame(t.Context(), exe, title)
	if err != nil {
		t.Fatalf("add %s: %v", title, err)
	}
	return game
}

func requireNoRemovalStaging(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(dir + removingSuffix + "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("folder renamed for removal although the removal was refused: %v", matches)
	}
}

func requireJournalEmpty(t *testing.T, s *Service) {
	t.Helper()
	pending, err := s.removals.load()
	if err != nil {
		t.Fatalf("load removals: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("removal journal = %v, want empty", pending)
	}
}

func TestRemoveGameRefusesAFolderAnotherGameShares(t *testing.T) {
	type layout struct {
		a, b string
	}
	cases := []struct {
		name    string
		layout  layout
		prepare func(t *testing.T, r sharedRig, bExe string)
		// want is the code of the refusal; empty means the removal goes ahead.
		want string
	}{
		{"same folder", layout{"RepackX", "RepackX/config.exe"}, addActive, "install.folder_shared"},
		{"other game inside", layout{"RepackX", "RepackX/Bonus/Game2.exe"}, addActive, "install.folder_shared"},
		{"game inside the other", layout{"Parent/RepackX", "Parent/launcher.exe"}, addActive, "install.folder_shared"},
		{"siblings", layout{"Games/RepackX", "Games/Other/other.exe"}, addActive, ""},
		{"sibling whose name starts with the folder's", layout{"Games/RepackX", "Games/RepackXBonus/b.exe"}, addActive, ""},
		{"sibling whose name the folder's starts with", layout{"Games/RepackX", "Games/Repack/b.exe"}, addActive, ""},
		{"archived card", layout{"RepackX", "RepackX/Bonus/Game2.exe"}, addArchived, ""},
		{"uninstalled card", layout{"RepackX", "RepackX/Bonus/Game2.exe"}, addUninstalled, ""},
		{"emulated game keeps its rule", layout{"RepackX", "RepackX/psx/game.cue"}, addEmulated, uierr.Code(emulators.ErrImageInside)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newSharedRig(t)
			root := t.TempDir()
			aDir := filepath.Join(root, filepath.FromSlash(tc.layout.a))
			bExe := filepath.Join(root, filepath.FromSlash(tc.layout.b))
			a := r.ownedGame(t, aDir, "RepackX", library.Uninstall{})
			tc.prepare(t, r, bExe)

			err := r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true})

			if tc.want == "" {
				if err != nil {
					t.Fatalf("RemoveGame = %v, want the removal to go ahead", err)
				}
				if !missing(t, aDir) {
					t.Fatal("the game folder is still there")
				}
				return
			}
			if got := uierr.Code(err); got != tc.want {
				t.Fatalf("RemoveGame code = %q (%v), want %q", got, err, tc.want)
			}
			if missing(t, filepath.Join(aDir, "RepackX.exe")) || missing(t, bExe) {
				t.Fatal("files deleted although the removal was refused")
			}
			requireNoRemovalStaging(t, aDir)
			requireJournalEmpty(t, r.s)
			if game, err := r.lib.Find(a.ID); err != nil || game.Uninstalled {
				t.Fatalf("card of the refused game = %+v, %v; want it installed", game, err)
			}
			info, err := r.s.InspectRemoval(a.ID)
			if err != nil {
				t.Fatalf("InspectRemoval: %v", err)
			}
			if len(info.SharedWith) != 1 || info.SharedWith[0].Title != "Bonus" {
				t.Fatalf("SharedWith = %+v, want the other game named", info.SharedWith)
			}
		})
	}
}

func addActive(t *testing.T, r sharedRig, exe string) {
	t.Helper()
	r.addedGame(t, exe, "Bonus")
}

func addArchived(t *testing.T, r sharedRig, exe string) {
	t.Helper()
	b := r.addedGame(t, exe, "Bonus")
	if err := r.lib.RemoveGame(b.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
}

func addUninstalled(t *testing.T, r sharedRig, exe string) {
	t.Helper()
	b := r.addedGame(t, exe, "Bonus")
	if err := r.lib.MarkUninstalled(b.ID); err != nil {
		t.Fatalf("mark uninstalled: %v", err)
	}
}

func addEmulated(t *testing.T, r sharedRig, cue string) {
	t.Helper()
	mkText(t, filepath.Join(filepath.Dir(cue), "game.bin"), "12345678")
	mkText(t, cue, `FILE "game.bin" BINARY`)
	if _, err := r.lib.AddEmulatedGame(t.Context(), cue, emulators.PS1, "Bonus"); err != nil {
		t.Fatalf("add emulated: %v", err)
	}
}

func TestRemoveGameFromLibraryKeepsASharedFolder(t *testing.T) {
	r := newSharedRig(t)
	aDir := filepath.Join(t.TempDir(), "RepackX")
	a := r.ownedGame(t, aDir, "RepackX", library.Uninstall{})
	bExe := filepath.Join(aDir, "Bonus", "Game2.exe")
	b := r.addedGame(t, bExe, "Bonus")

	if err := r.s.RemoveGame(a.ID, RemoveOptions{}); err != nil {
		t.Fatalf("RemoveGame without files = %v", err)
	}
	if _, err := r.lib.Find(a.ID); !library.IsGameNotFound(err) {
		t.Fatalf("card of the removed game: %v", err)
	}
	if missing(t, filepath.Join(aDir, "RepackX.exe")) || missing(t, bExe) {
		t.Fatal("files deleted by a removal from the library")
	}
	if _, err := r.lib.Find(b.ID); err != nil {
		t.Fatalf("card of the other game: %v", err)
	}
}

// The removal decides with the library's cards in view and the other game is
// added at the same moment: whichever comes first, a card never points at
// files that were deleted with another game.
func TestRemovalAndAddingAGameInsideItNeverLoseTheAddedGame(t *testing.T) {
	for i := range 20 {
		r := newSharedRig(t)
		aDir := filepath.Join(t.TempDir(), "RepackX")
		a := r.ownedGame(t, aDir, "RepackX", library.Uninstall{})
		bExe := filepath.Join(aDir, "Bonus", "Game2.exe")
		mkFile(t, bExe, 512)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var removeErr, addErr error
		var b library.Game
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			removeErr = r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true})
		}()
		go func() {
			defer wg.Done()
			<-start
			b, addErr = r.lib.AddGame(context.Background(), bExe, "Bonus")
		}()
		close(start)
		wg.Wait()

		_, statErr := os.Stat(bExe)
		filesThere := statErr == nil
		if addErr == nil {
			if _, err := r.lib.Find(b.ID); err != nil {
				t.Fatalf("round %d: AddGame succeeded but the card is gone: %v", i, err)
			}
			if !filesThere {
				t.Fatalf("round %d: the added game's card points at files deleted with another game (remove: %v)", i, removeErr)
			}
			if removeErr == nil {
				t.Fatalf("round %d: the removal went ahead although a game was added inside its folder", i)
			}
		}
		if removeErr != nil {
			if code := uierr.Code(removeErr); code != "install.folder_shared" {
				t.Fatalf("round %d: removal refused with %q (%v), want install.folder_shared", i, code, removeErr)
			}
			if missing(t, filepath.Join(aDir, "RepackX.exe")) {
				t.Fatalf("round %d: refused removal deleted files", i)
			}
		}
	}
}

func TestRemoveGameRefusedWhileTheGameIsBeingMoved(t *testing.T) {
	cases := []struct {
		name      string
		opts      RemoveOptions
		installer bool
	}{
		{"delete files", RemoveOptions{DeleteFiles: true}, false},
		{"delete files and keep the card", RemoveOptions{DeleteFiles: true, KeepInLibrary: true}, false},
		{"library only", RemoveOptions{}, false},
		{"uninstaller", RemoveOptions{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newSharedRig(t)
			runner := &fakeRunner{}
			r.s.runner = runner
			dir := filepath.Join(t.TempDir(), "Moving")
			var uninstall library.Uninstall
			if tc.installer {
				uninstaller := filepath.Join(dir, "unins000.exe")
				mkFile(t, uninstaller, 512)
				uninstall = library.Uninstall{Command: uninstaller}
			}
			a := r.ownedGame(t, dir, "Moving", uninstall)
			release, err := r.lib.BeginRelocate(a.ID)
			if err != nil {
				t.Fatalf("take the move reservation: %v", err)
			}

			err = r.s.RemoveGame(a.ID, tc.opts)

			if code := uierr.Code(err); code != "install.game_moving" {
				t.Fatalf("RemoveGame during a move = %q (%v), want install.game_moving", code, err)
			}
			if calls := runner.calls(); len(calls) != 0 {
				t.Fatalf("uninstaller ran during a move: %+v", calls)
			}
			if missing(t, filepath.Join(dir, "Moving.exe")) {
				t.Fatal("files deleted during a move")
			}
			requireNoRemovalStaging(t, dir)
			if game, err := r.lib.Find(a.ID); err != nil || game.Uninstalled {
				t.Fatalf("card during a move = %+v, %v; want it installed", game, err)
			}

			release()
			if err := r.s.RemoveGame(a.ID, tc.opts); err != nil {
				t.Fatalf("RemoveGame after the move ended = %v", err)
			}
		})
	}
}

// The recovery of another game's move writes its copy into a folder inside
// this game's: deleting this game's folder would delete that copy, which may
// be the only one. The removal is refused with the library's code, and
// nothing is deleted or staged.
func TestRemoveGameRefusedAroundAnotherGamesRecoveringCopy(t *testing.T) {
	r := newSharedRig(t)
	dir := filepath.Join(t.TempDir(), "Tools")
	tool := r.ownedGame(t, dir, "Tool", library.Uninstall{})
	moving := r.addedGame(t, filepath.Join(t.TempDir(), "Game", "game.exe"), "Game")
	copyDir := filepath.Join(dir, "Game")
	mkFile(t, filepath.Join(copyDir, "game.exe"), 512)
	release, err := r.lib.BeginRecovery(moving.ID, moving.InstallDir, copyDir, copyDir+".staging")
	if err != nil {
		t.Fatalf("take the recovery reservation: %v", err)
	}

	err = r.s.RemoveGame(tool.ID, RemoveOptions{DeleteFiles: true})

	if code := uierr.Code(err); code != "library.folder_busy" {
		t.Fatalf("RemoveGame around a recovering copy = %q (%v), want library.folder_busy", code, err)
	}
	if missing(t, filepath.Join(copyDir, "game.exe")) {
		t.Fatal("the recovering copy was deleted with the folder around it")
	}
	requireNoRemovalStaging(t, dir)
	requireJournalEmpty(t, r.s)
	release()
	if err := r.s.RemoveGame(tool.ID, RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("RemoveGame after the recovery ended = %v", err)
	}
}

// A move asks the library for the game while the uninstaller is running: the
// removal holds the game from its decision to its end.
func TestMoveRefusedWhileTheGameIsBeingRemoved(t *testing.T) {
	r := newSharedRig(t)
	entered := make(chan struct{})
	proceed := make(chan struct{})
	r.s.runner = &fakeRunner{act: func(runSpec) {
		close(entered)
		<-proceed
	}}
	dir := filepath.Join(t.TempDir(), "Leaving")
	uninstaller := filepath.Join(dir, "unins000.exe")
	mkFile(t, uninstaller, 512)
	a := r.ownedGame(t, dir, "Leaving", library.Uninstall{Command: uninstaller})

	done := make(chan error, 1)
	go func() { done <- r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true, KeepInLibrary: true}) }()
	<-entered
	release, err := r.lib.BeginRelocate(a.ID)
	if err == nil {
		release()
		close(proceed)
		if err := <-done; err != nil {
			t.Errorf("RemoveGame = %v", err)
		}
		t.Fatal("a move took the game while it was being removed")
	}
	if code := uierr.Code(err); code != "library.removing" {
		t.Errorf("move reservation during a removal = %q (%v), want library.removing", code, err)
	}
	close(proceed)
	if err := <-done; err != nil {
		t.Fatalf("RemoveGame = %v", err)
	}
	release, err = r.lib.BeginRelocate(a.ID)
	if err != nil {
		t.Fatalf("the removal kept its reservation after it ended: %v", err)
	}
	release()
}

// The dialog sends these for a game with an uninstaller: "remove from computer"
// on a game the launcher does not own keeps the card, "remove from library"
// with the files toggle off does not. The uninstaller runs in the game folder
// either way, and it may take the whole folder with it.
var uninstallerRuns = []struct {
	name string
	opts RemoveOptions
}{
	{"keep the card", RemoveOptions{KeepInLibrary: true}},
	{"remove the card", RemoveOptions{}},
}

// The uninstaller runs for every removal of such a game, so the refusal has a
// code of its own: its text cannot advise "remove from the library only",
// which would run the same uninstaller.
func TestUninstallerRefusedInAFolderAnotherGameShares(t *testing.T) {
	runs := append([]struct {
		name string
		opts RemoveOptions
	}{{"delete the files", RemoveOptions{DeleteFiles: true}}}, uninstallerRuns...)
	for _, tc := range runs {
		t.Run(tc.name, func(t *testing.T) {
			r := newSharedRig(t)
			dir := filepath.Join(t.TempDir(), "RepackX")
			uninstaller := filepath.Join(dir, "unins000.exe")
			mkFile(t, uninstaller, 512)
			runner := &fakeRunner{act: func(runSpec) {
				if err := os.RemoveAll(dir); err != nil {
					t.Error(err)
				}
			}}
			r.s.runner = runner
			a := r.ownedGame(t, dir, "RepackX", library.Uninstall{Command: uninstaller})
			bExe := filepath.Join(dir, "Bonus", "Game2.exe")
			b := r.addedGame(t, bExe, "Bonus")

			err := r.s.RemoveGame(a.ID, tc.opts)

			if code := uierr.Code(err); code != "install.folder_shared_uninstaller" {
				t.Fatalf("RemoveGame = %q (%v), want install.folder_shared_uninstaller", code, err)
			}
			if calls := runner.calls(); len(calls) != 0 {
				t.Fatalf("the uninstaller ran in a folder another game shares: %+v", calls)
			}
			if missing(t, bExe) {
				t.Fatal("the other game's files are gone")
			}
			if game, err := r.lib.Find(a.ID); err != nil || game.Uninstalled {
				t.Fatalf("card of the refused game = %+v, %v; want it installed", game, err)
			}
			if _, err := r.lib.Find(b.ID); err != nil {
				t.Fatalf("card of the other game: %v", err)
			}
		})
	}
}

func TestUninstallerHoldsTheFolderWhileItRuns(t *testing.T) {
	for _, tc := range uninstallerRuns {
		t.Run(tc.name, func(t *testing.T) {
			r := newSharedRig(t)
			dir := filepath.Join(t.TempDir(), "RepackX")
			uninstaller := filepath.Join(dir, "unins000.exe")
			mkFile(t, uninstaller, 512)
			late := filepath.Join(dir, "Late", "Game3.exe")
			mkFile(t, late, 512)
			var addErr error
			r.s.runner = &fakeRunner{act: func(runSpec) {
				_, addErr = r.lib.AddGame(t.Context(), late, "Late")
			}}
			a := r.ownedGame(t, dir, "RepackX", library.Uninstall{Command: uninstaller})

			if err := r.s.RemoveGame(a.ID, tc.opts); err != nil {
				t.Fatalf("RemoveGame = %v", err)
			}
			if code := uierr.Code(addErr); code != "library.folder_busy" {
				t.Fatalf("AddGame inside the folder while its uninstaller ran = %q (%v), want library.folder_busy", code, addErr)
			}
			if games := r.lib.GetGames(); len(games) > 1 {
				t.Fatalf("cards = %+v, want no card added inside the folder", games)
			}
		})
	}
}

// The removal keeps its hold until the card is settled: a game added inside
// the folder after the files went, before the card changed, is refused.
func TestRemovalHoldsTheFolderUntilTheCardIsSettled(t *testing.T) {
	r := newSharedRig(t)
	dir := filepath.Join(t.TempDir(), "RepackX")
	a := r.ownedGame(t, dir, "RepackX", library.Uninstall{})
	late := filepath.Join(dir, "Late", "Game3.exe")
	var addErr error
	r.s.releaseRuntime = func(string) error {
		mkFile(t, late, 64)
		_, addErr = r.lib.AddGame(t.Context(), late, "Late")
		return nil
	}

	if err := r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("RemoveGame = %v", err)
	}
	if code := uierr.Code(addErr); code != "library.folder_busy" {
		t.Fatalf("AddGame inside the folder after its files went = %q (%v), want library.folder_busy", code, addErr)
	}
	if games := r.lib.GetGames(); len(games) != 0 {
		t.Fatalf("cards = %+v, want none", games)
	}
}

// The card's folder changes between the snapshot the removal plans with and
// its reservation, by a move that ended or by a scan that found the game
// elsewhere: the removal acts on the folder it reserved or on none, never on
// one the card no longer names while it settles the card. Nothing moves the
// game any more, so the refusal says the folder changed, not that it moves.
func TestRemovalActsOnTheFolderItPlannedWith(t *testing.T) {
	changes := map[string]func(t *testing.T, r sharedRig, id, oldDir, newDir string){
		"a move ended": func(t *testing.T, r sharedRig, id, oldDir, newDir string) {
			release, err := r.lib.BeginMove(id)
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			if err := os.Rename(oldDir, newDir); err != nil {
				t.Error(err)
			}
			if _, err := r.lib.Relocate(id, newDir); err != nil {
				t.Error(err)
			}
		},
		"a scan found it elsewhere": func(t *testing.T, r sharedRig, id, _, newDir string) {
			exe := filepath.Join(newDir, "Game.exe")
			mkFile(t, exe, 64)
			if _, _, err := r.lib.ApplyDiscovered(library.Discovered{GameID: id, Title: "Game", Executable: exe, InstallDir: newDir}); err != nil {
				t.Error(err)
			}
		},
	}
	for cause, change := range changes {
		for _, keep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/keep=%v", cause, keep), func(t *testing.T) {
				r := newSharedRig(t)
				oldDir := filepath.Join(t.TempDir(), "Game")
				newDir := filepath.Join(t.TempDir(), "Game")
				a := r.ownedGame(t, oldDir, "Game", library.Uninstall{})
				var once sync.Once
				r.s.SetBusyCheck(func(string) bool {
					once.Do(func() { change(t, r, a.ID, oldDir, newDir) })
					return false
				})

				err := r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true, KeepInLibrary: keep})

				requireFolderChangedRefusal(t, r, a.ID, err, oldDir, newDir)
			})
		}
	}
}

func requireFolderChangedRefusal(t *testing.T, r sharedRig, id string, err error, oldDir, newDir string) {
	t.Helper()
	if code := uierr.Code(err); code != "install.folder_changed" {
		t.Fatalf("RemoveGame after the folder changed = %q (%v), want install.folder_changed", code, err)
	}
	if missing(t, filepath.Join(newDir, "Game.exe")) {
		t.Fatal("files at the new folder deleted")
	}
	card, err := r.lib.Find(id)
	if err != nil || card.Uninstalled || !samePath(card.InstallDir, newDir) {
		t.Fatalf("card = %+v, %v; want it installed at %s", card, err, newDir)
	}
	requireNoRemovalStaging(t, oldDir)
	requireNoRemovalStaging(t, newDir)
	requireJournalEmpty(t, r.s)
}

func TestRemovalReleasesTheGameWhenItFails(t *testing.T) {
	r := newSharedRig(t)
	r.s.runner = &fakeRunner{code: 5}
	dir := filepath.Join(t.TempDir(), "Stays")
	uninstaller := filepath.Join(dir, "unins000.exe")
	mkFile(t, uninstaller, 512)
	a := r.ownedGame(t, dir, "Stays", library.Uninstall{Command: uninstaller})

	if err := r.s.RemoveGame(a.ID, RemoveOptions{DeleteFiles: true}); uierr.Code(err) != "install.uninstall_failed" {
		t.Fatalf("RemoveGame = %v, want install.uninstall_failed", err)
	}
	release, err := r.lib.BeginRelocate(a.ID)
	if err != nil {
		t.Fatalf("the failed removal kept its reservation: %v", err)
	}
	release()
}
