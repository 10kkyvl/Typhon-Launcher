package relocate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"typhon/internal/emulators"
	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

func addExeAt(t *testing.T, lib *library.Service, exe, title string) library.Game {
	t.Helper()
	mustMkdirFile(t, filepath.Dir(exe), filepath.Base(exe), "stub")
	game, err := lib.AddGame(t.Context(), exe, title)
	if err != nil {
		t.Fatalf("add %s: %v", title, err)
	}
	return game
}

func TestMoveGameRefusesAFolderAnotherGameShares(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		prepare func(t *testing.T, lib *library.Service, b string)
		// want is the code of the refusal; empty means the move goes ahead.
		want string
	}{
		{"same folder", "RepackX", "RepackX/config.exe", neighbourActive, "relocate.folder_shared"},
		{"other game inside", "RepackX", "RepackX/Bonus/Game2.exe", neighbourActive, "relocate.folder_shared"},
		{"game inside the other", "Parent/RepackX", "Parent/launcher.exe", neighbourActive, "relocate.folder_shared"},
		{"image of an emulated game inside", "RepackX", "RepackX/psx/game.cue", neighbourEmulated, "relocate.folder_shared"},
		{"siblings", "Games/RepackX", "Games/Other/other.exe", neighbourActive, ""},
		{"sibling whose name starts with the folder's", "Games/RepackX", "Games/RepackXBonus/b.exe", neighbourActive, ""},
		{"sibling whose name the folder's starts with", "Games/RepackX", "Games/Repack/b.exe", neighbourActive, ""},
		{"archived card", "RepackX", "RepackX/Bonus/Game2.exe", neighbourArchived, ""},
		{"uninstalled card", "RepackX", "RepackX/Bonus/Game2.exe", neighbourUninstalled, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			s := newTestService(t, nil, lib, nil, nil, nil)
			root := t.TempDir()
			aExe := filepath.Join(root, filepath.FromSlash(tc.a), "game.exe")
			a := addExeAt(t, lib, aExe, "RepackX")
			bPath := filepath.Join(root, filepath.FromSlash(tc.b))
			tc.prepare(t, lib, bPath)
			parent := t.TempDir()
			target := filepath.Join(parent, "RepackX")

			job, err := s.MoveGame(a.ID, parent)

			if tc.want == "" {
				if err != nil {
					t.Fatalf("MoveGame = %v, want the move to go ahead", err)
				}
				s.wait(job.ID)
				if _, err := os.Stat(filepath.Join(target, "game.exe")); err != nil {
					t.Fatalf("the game did not arrive: %v", err)
				}
				return
			}
			if got := uierr.Code(err); got != tc.want {
				t.Fatalf("MoveGame code = %q (%v), want %q", got, err, tc.want)
			}
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("job registered despite the refusal: %+v", jobs)
			}
			if _, err := os.Stat(aExe); err != nil {
				t.Fatalf("source touched despite the refusal: %v", err)
			}
			if _, err := os.Stat(bPath); err != nil {
				t.Fatalf("the other game's file touched despite the refusal: %v", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("target created despite the refusal: %v", err)
			}
		})
	}
}

// A library move refuses a game whose folder holds another library game before
// it registers a job or moves anything, as a single game's move does.
func TestMoveLibraryRefusesASharedFolderBeforeMoving(t *testing.T) {
	set := newTestSettings(t)
	lib := newTestLibrary(t)
	s := newTestService(t, set, lib, nil, nil, nil)
	cfg := set.GetSettings()
	a := addGameAt(t, lib, cfg.GamesPath, "RepackX")
	bonus := filepath.Join(a.InstallDir, "Bonus", "Game2.exe")
	addExeAt(t, lib, bonus, "Bonus")

	_, err := s.MoveLibrary(t.TempDir())

	if code := uierr.Code(err); code != "relocate.folder_shared" {
		t.Fatalf("MoveLibrary = %q (%v), want relocate.folder_shared", code, err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job registered despite the refusal: %+v", got)
	}
	if _, err := os.Stat(bonus); err != nil {
		t.Fatalf("the other game's file touched: %v", err)
	}
}

func neighbourActive(t *testing.T, lib *library.Service, exe string) {
	t.Helper()
	addExeAt(t, lib, exe, "Bonus")
}

func neighbourArchived(t *testing.T, lib *library.Service, exe string) {
	t.Helper()
	b := addExeAt(t, lib, exe, "Bonus")
	if err := lib.RemoveGame(b.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
}

func neighbourUninstalled(t *testing.T, lib *library.Service, exe string) {
	t.Helper()
	b := addExeAt(t, lib, exe, "Bonus")
	if err := lib.MarkUninstalled(b.ID); err != nil {
		t.Fatalf("mark uninstalled: %v", err)
	}
}

func neighbourEmulated(t *testing.T, lib *library.Service, cue string) {
	t.Helper()
	mustMkdirFile(t, filepath.Dir(cue), "game.bin", "12345678")
	mustMkdirFile(t, filepath.Dir(cue), filepath.Base(cue), `FILE "game.bin" BINARY`)
	if _, err := lib.AddEmulatedGame(t.Context(), cue, emulators.PS1, "Bonus"); err != nil {
		t.Fatalf("add emulated: %v", err)
	}
}

// removalAttempt is what the library answers to the two ways a game can leave
// it while its files are being moved.
type removalAttempt struct {
	stage       string
	remove      error
	uninstalled error
}

func tryToRemove(lib *library.Service, id, stage string) removalAttempt {
	return removalAttempt{stage: stage, remove: lib.RemoveGame(id), uninstalled: lib.MarkUninstalled(id)}
}

func TestGameCannotLeaveTheLibraryDuringAMove(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := t.TempDir()

	var attempts []removalAttempt
	prevRename, prevRemove := renameDir, removeTree
	renameDir = func(oldpath, newpath string) error {
		attempts = append(attempts, tryToRemove(lib, game.ID, "prepare"))
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errCrossDevice}
	}
	afterCopyBeforeVerify = func(string) {
		attempts = append(attempts, tryToRemove(lib, game.ID, "copy"))
	}
	removeTree = func(path string) error {
		attempts = append(attempts, tryToRemove(lib, game.ID, "cleanup"))
		return os.RemoveAll(path)
	}
	t.Cleanup(func() {
		renameDir, removeTree, afterCopyBeforeVerify = prevRename, prevRemove, nil
	})

	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatalf("MoveGame: %v", err)
	}
	s.wait(job.ID)

	for _, a := range attempts {
		if code := uierr.Code(a.remove); code != "library.moving" {
			t.Errorf("RemoveGame during the %s stage = %q (%v), want library.moving", a.stage, code, a.remove)
		}
		if code := uierr.Code(a.uninstalled); code != "library.moving" {
			t.Errorf("MarkUninstalled during the %s stage = %q (%v), want library.moving", a.stage, code, a.uninstalled)
		}
	}
	if len(attempts) != 3 {
		t.Fatalf("pause points reached = %d, want 3", len(attempts))
	}
	moved, err := lib.Find(game.ID)
	if err != nil || moved.Uninstalled {
		t.Fatalf("card after the move = %+v, %v; want it installed", moved, err)
	}
	if !samePathT(moved.InstallDir, filepath.Join(parent, filepath.Base(src))) {
		t.Fatalf("card points at %s after the move", moved.InstallDir)
	}
	if err := lib.MarkUninstalled(game.ID); err != nil {
		t.Fatalf("the move kept its reservation after it ended: %v", err)
	}
}

func samePathT(a, b string) bool {
	ca, errA := filepath.Abs(a)
	cb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(ca) == filepath.Clean(cb)
}

func TestMoveReleasesTheGameWhenCancelledOrFailed(t *testing.T) {
	cases := []struct {
		name string
		hook func(t *testing.T, s *Service, staging string)
		want Stage
	}{
		{"cancelled", func(t *testing.T, s *Service, _ string) {
			for _, j := range s.List() {
				if err := s.Cancel(j.ID); err != nil {
					t.Error(err)
				}
			}
		}, StageCancelled},
		{"verify failed", func(t *testing.T, _ *Service, staging string) {
			if err := os.WriteFile(filepath.Join(staging, "game.exe"), []byte("corrupted"), 0o600); err != nil {
				t.Error(err)
			}
		}, StageFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forceCopyPath(t)
			lib := newTestLibrary(t)
			s := newTestService(t, nil, lib, nil, nil, nil)
			game, _ := addTestGame(t, lib)
			afterCopyBeforeVerify = func(staging string) { tc.hook(t, s, staging) }
			t.Cleanup(func() { afterCopyBeforeVerify = nil })

			job, err := s.MoveGame(game.ID, t.TempDir())
			if err != nil {
				t.Fatalf("MoveGame: %v", err)
			}
			if got := s.wait(job.ID); got.Stage != tc.want {
				t.Fatalf("stage = %s, want %s", got.Stage, tc.want)
			}
			if err := lib.MarkUninstalled(game.ID); err != nil {
				t.Fatalf("the %s move kept its reservation: %v", tc.name, err)
			}
		})
	}
}

// A copy that reached the target is verified data nobody asked to delete. When
// its card is gone before the repoint, nothing is deleted and the job stays in
// the journal across starts, so the copy stays findable until it is gone.
func TestCopyOfAGameThatLeftTheLibraryStaysTracked(t *testing.T) {
	cases := []struct {
		name  string
		leave func(lib *library.Service, id string) error
	}{
		{"removed from the library", func(lib *library.Service, id string) error { return lib.RemoveGame(id) }},
		{"uninstalled", func(lib *library.Service, id string) error { return lib.MarkUninstalled(id) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "Game")
			mustMkdirFile(t, target, "game.exe", "stub")
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.leave(lib, game.ID); err != nil {
				t.Fatal(err)
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: StageRepoint, GameID: game.ID, Title: "Game", Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}
			srcBefore, targetBefore := snapshotTree(t, src), snapshotTree(t, target)

			s.recoverAll(context.Background())
			first := s.soleJob(t)
			if first.Stage != StageFailed {
				t.Fatalf("job after the first start = %+v", first)
			}
			if !strings.Contains(first.Error, "relocate.game_removed") {
				t.Fatalf("job error %q does not carry relocate.game_removed", first.Error)
			}

			s.recoverAll(context.Background())
			s.soleJob(t)
			if !reflect.DeepEqual(srcBefore, snapshotTree(t, src)) || !reflect.DeepEqual(targetBefore, snapshotTree(t, target)) {
				t.Fatal("recovery touched the copies of a game that left the library")
			}
			if card, err := lib.Find(game.ID); err == nil && !card.Uninstalled {
				t.Fatalf("the card came back as installed: %+v", card)
			} else if err == nil && samePathT(card.InstallDir, target) {
				t.Fatalf("an uninstalled card was pointed at the copy: %+v", card)
			}
			copies, err := s.OrphanedCopies()
			if err != nil {
				t.Fatal(err)
			}
			if len(copies) != 1 || !samePathT(copies[0].Path, target) || copies[0].GameID != game.ID {
				t.Fatalf("OrphanedCopies = %+v, want the copy at %s", copies, target)
			}

			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			s.recoverAll(context.Background())
			if jobs := s.List(); len(jobs) != 0 {
				t.Fatalf("job kept after its copy is gone: %+v", jobs)
			}
		})
	}
}

// A live move holds its target and its staging folder as a recovery holds its
// copies, from before anything is made there until its goroutine ends: a
// removal with files of a card whose folder holds one of them is refused (just
// after a same-volume rename the target is the only copy), and no card is
// added at or inside them. MoveGame and a library move's game item alike.
func TestLiveMoveHoldsItsTargetAndStaging(t *testing.T) {
	type attempt struct {
		what string
		err  error
	}
	cases := []struct {
		name    string
		library bool
		copy    bool
	}{
		{"move, rename", false, false},
		{"move, copy", false, true},
		{"library move item, rename", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.copy {
				forceCopyPath(t)
			}
			var set *settings.Service
			gamesDir := t.TempDir()
			if tc.library {
				set = newTestSettings(t)
				gamesDir = set.GetSettings().GamesPath
			}
			lib := newTestLibrary(t)
			s := newTestService(t, set, lib, nil, nil, nil)
			game := addGameAt(t, lib, gamesDir, "Game")
			parent := t.TempDir()
			// A store's card is not moved by a library move, so the Tool stays
			// in the folder that holds the new library.
			toolExe := mustMkdirFile(t, parent, "tool.exe", "stub")
			if _, err := lib.ApplyExternal(t.Context(), []library.ExternalGame{{Provider: library.ProviderSteam, AppID: "42", Title: "Tool", InstallDir: parent, Executable: toolExe}}); err != nil {
				t.Fatal(err)
			}
			var tool library.Game
			for _, g := range lib.GetGames() {
				if g.External != nil {
					tool = g
				}
			}

			var attempts []attempt
			try := func(where string) {
				release, err := lib.BeginRemoval(tool.ID, tool.InstallDir)
				if err == nil {
					release()
				}
				attempts = append(attempts, attempt{"removal of the folder around the " + where, err})
				_, err = lib.AddGame(t.Context(), filepath.Join(where, "late.exe"), "Late")
				attempts = append(attempts, attempt{"card added inside the " + where, err})
			}
			prev := renameDir
			renameDir = func(oldpath, newpath string) error {
				if !tc.copy && samePathT(oldpath, game.InstallDir) {
					if err := prev(oldpath, newpath); err != nil {
						return err
					}
					mustMkdirFile(t, newpath, "late.exe", "stub")
					try(newpath)
					return os.Remove(filepath.Join(newpath, "late.exe"))
				}
				return prev(oldpath, newpath)
			}
			afterCopyBeforeVerify = func(staging string) {
				late := mustMkdirFile(t, staging+"-late", "late.exe", "stub")
				if err := os.Rename(late, filepath.Join(staging, "late.exe")); err != nil {
					t.Error(err)
				}
				try(staging)
				if err := os.Remove(filepath.Join(staging, "late.exe")); err != nil {
					t.Error(err)
				}
			}
			t.Cleanup(func() { renameDir, afterCopyBeforeVerify = prev, nil })

			var job Job
			var err error
			if tc.library {
				job, err = s.MoveLibrary(parent)
			} else {
				job, err = s.MoveGame(game.ID, parent)
			}
			if err != nil {
				t.Fatalf("move = %v", err)
			}
			done := s.wait(job.ID)
			if done.Stage != "" && done.Stage != StageDone {
				t.Fatalf("move = %s (%s)", done.Stage, done.Error)
			}
			if len(attempts) == 0 {
				t.Fatal("the move never reached its target or staging folder")
			}
			for _, a := range attempts {
				if code := uierr.Code(a.err); code != "library.folder_busy" {
					t.Errorf("%s during the move = %q (%v), want library.folder_busy", a.what, code, a.err)
				}
			}
			card, err := lib.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(card.InstallDir, "game.exe")); err != nil {
				t.Fatalf("the moved game: %v", err)
			}
			release, err := lib.BeginRemoval(game.ID, "")
			if err != nil {
				t.Fatalf("the game after the move ended = %v, want it free", err)
			}
			release()
		})
	}
}

// A move into a folder another game's removal is deleting is refused before
// anything is made at its target, and the refusal leaves nothing behind.
func TestMoveIntoAFolderBeingRemovedIsRefusedWithNothingMade(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := filepath.Join(t.TempDir(), "Tools")
	tool := addExeAt(t, lib, filepath.Join(parent, "tool.exe"), "Tool")
	release, err := lib.BeginRemoval(tool.ID, tool.InstallDir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, err = s.MoveGame(game.ID, filepath.Join(parent, "Games"))

	if code := uierr.Code(err); code != "library.folder_busy" {
		t.Fatalf("MoveGame into a folder being removed = %q (%v), want library.folder_busy", code, err)
	}
	if _, err := os.Stat(filepath.Join(parent, "Games")); !os.IsNotExist(err) {
		t.Fatalf("the refused move made its target's parent: %v", err)
	}
	if jobs := s.List(); len(jobs) != 0 {
		t.Fatalf("jobs = %+v, want none", jobs)
	}
	if card, err := lib.Find(game.ID); err != nil || !samePathT(card.InstallDir, src) {
		t.Fatalf("card = %+v, %v", card, err)
	}
}

// The move decides with the library's cards in view and a game is added inside
// its folder at the same moment: a card never ends up pointing at a file the
// move carried away.
func TestMoveAndAddingAGameInsideItNeverLoseTheAddedGame(t *testing.T) {
	for i := range 15 {
		lib := newTestLibrary(t)
		s := newTestService(t, nil, lib, nil, nil, nil)
		a, src := addTestGame(t, lib)
		bExe := filepath.Join(src, "Bonus", "Game2.exe")
		mustMkdirFile(t, filepath.Dir(bExe), filepath.Base(bExe), "stub")
		parent := t.TempDir()

		start := make(chan struct{})
		var wg sync.WaitGroup
		var moveErr, addErr error
		var job Job
		var b library.Game
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			job, moveErr = s.MoveGame(a.ID, parent)
		}()
		go func() {
			defer wg.Done()
			<-start
			b, addErr = lib.AddGame(context.Background(), bExe, "Bonus")
		}()
		close(start)
		wg.Wait()
		if moveErr == nil {
			s.wait(job.ID)
		}

		if addErr == nil {
			if moveErr == nil {
				t.Fatalf("round %d: the move went ahead although a game was added inside its folder", i)
			}
			if code := uierr.Code(moveErr); code != "relocate.folder_shared" {
				t.Fatalf("round %d: move refused with %q (%v), want relocate.folder_shared", i, code, moveErr)
			}
			card, err := lib.Find(b.ID)
			if err != nil {
				t.Fatalf("round %d: card of the added game: %v", i, err)
			}
			if _, err := os.Stat(card.Executable); err != nil {
				t.Fatalf("round %d: the added game's card points at a file the move carried away: %v", i, err)
			}
		}
	}
}
