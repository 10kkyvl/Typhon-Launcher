package relocate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeBusy struct{ busy bool }

func (f fakeBusy) Busy(string) bool { return f.busy }

func newTestLibrary(t *testing.T) *library.Service {
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
	return lib
}

func newTestSettings(t *testing.T) *settings.Service {
	t.Helper()
	set, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("new settings: %v", err)
	}
	if _, err := set.SetupLibrary(t.TempDir()); err != nil {
		t.Fatalf("setup library: %v", err)
	}
	return set
}

func newTestService(t *testing.T, set *settings.Service, lib *library.Service, dl *download.Manager, inst, upd busyChecker) *Service {
	t.Helper()
	s, err := NewServiceAt(t.TempDir(), set, lib, dl, inst, upd)
	if err != nil {
		t.Fatalf("new relocate service: %v", err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("relocate shutdown: %v", err)
		}
	})
	return s
}

func addGameAt(t *testing.T, lib *library.Service, dir, name string) library.Game {
	t.Helper()
	gameDir := filepath.Join(dir, name)
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(gameDir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	game, err := lib.AddGame(exe, name)
	if err != nil {
		t.Fatalf("add game %s: %v", name, err)
	}
	return game
}

func addTestGame(t *testing.T, lib *library.Service) (library.Game, string) {
	t.Helper()
	game := addGameAt(t, lib, t.TempDir(), "Game")
	return game, game.InstallDir
}

// unusedDriveTarget returns a path on a drive letter that does not exist,
// so platform.GetStorageInfo cannot resolve free space for it.
func unusedDriveTarget(t *testing.T) string {
	t.Helper()
	for _, letter := range "QXYZWVUT" {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); os.IsNotExist(err) {
			return root + `move-target`
		}
	}
	t.Skip("no unused drive letter available to force a storage-info failure")
	return ""
}

func TestMoveGameRefusesBusy(t *testing.T) {
	cases := []struct {
		name string
		inst busyChecker
		upd  busyChecker
		want error
	}{
		{"update busy", fakeBusy{false}, fakeBusy{true}, ErrUpdateBusy},
		{"install busy", fakeBusy{true}, fakeBusy{false}, ErrInstallBusy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			s := newTestService(t, nil, lib, nil, tc.inst, tc.upd)
			game, oldDir := addTestGame(t, lib)

			if _, err := s.MoveGame(game.ID, filepath.Join(t.TempDir(), "target")); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(oldDir, "game.exe")); err != nil {
				t.Fatalf("source touched despite refusal: %v", err)
			}
			if got := s.List(); len(got) != 0 {
				t.Fatalf("job registered despite refusal: %+v", got)
			}
		})
	}
}

func TestMoveGameRefusesUnknownGame(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	if _, err := s.MoveGame("missing-game-id", filepath.Join(t.TempDir(), "target")); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("err = %v, want ErrGameNotFound", err)
	}
}

func TestMoveGameRefusesEmptyInstallDir(t *testing.T) {
	// AddGame/RegisterInstalled never persist an empty InstallDir, so the
	// only way to observe one from outside the library package is a
	// pre-seeded library.json, the same way a partially-written record left
	// by an older bug would surface.
	path := filepath.Join(t.TempDir(), "library.json")
	raw := `[{"id":"g1","title":"Game","executable":"","installDir":""}]`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := library.NewServiceAt(path)
	if err != nil {
		t.Fatalf("new library: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	s := newTestService(t, nil, lib, nil, nil, nil)

	if _, err := s.MoveGame("g1", filepath.Join(t.TempDir(), "target")); !errors.Is(err, ErrEmptyInstallDir) {
		t.Fatalf("err = %v, want ErrEmptyInstallDir", err)
	}
}

func TestMoveGameRejectsBadTargets(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)

	root := filepath.VolumeName(oldDir) + `\`
	cases := []struct {
		name   string
		target string
		want   error
	}{
		{"empty", "", ErrEmptyTarget},
		{"relative", "relative/path", ErrRelativeTarget},
		{"volume root", root, ErrTargetIsRoot},
		{"inside source", filepath.Join(oldDir, "sub"), ErrTargetInsideSource},
		{"source inside target", filepath.Dir(oldDir), ErrSourceInsideTarget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.MoveGame(game.ID, tc.target); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMoveGameRefusesNonEmptyTarget(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)

	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "occupied.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveGame(game.ID, target); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	}
}

func TestMoveGameRefusesWhenFreeSpaceUnknown(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)
	target := unusedDriveTarget(t)

	if _, err := s.MoveGame(game.ID, target); !errors.Is(err, ErrFreeSpaceUnknown) {
		t.Fatalf("err = %v, want ErrFreeSpaceUnknown", err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "game.exe")); err != nil {
		t.Fatalf("source touched despite refusal: %v", err)
	}
}

func TestMoveGameRenameFastPath(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)
	target := filepath.Join(filepath.Dir(oldDir), "renamed-target")

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		t.Fatalf("move game: %v", err)
	}
	s.wait(job.ID)

	updated, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstallDir != target {
		t.Fatalf("installDir = %q, want %q", updated.InstallDir, target)
	}
	if _, err := os.Stat(filepath.Join(target, "game.exe")); err != nil {
		t.Fatalf("target missing executable: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir still present: %v", err)
	}
	if _, err := os.Stat(s.st.manifestPath(job.ID)); !os.IsNotExist(err) {
		t.Fatal("manifest created despite the rename fast path")
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("journal not cleared after completion: %+v", got)
	}
}

func TestMoveGameVerifyFailureKeepsSource(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)
	// A target whose parent does not exist yet makes os.Rename fail for a
	// mundane reason (ENOENT), forcing the copy/verify path without needing
	// an actual second volume.
	target := filepath.Join(t.TempDir(), "missing-parent", "game")

	prev := afterCopyBeforeVerify
	afterCopyBeforeVerify = func(staging string) {
		if err := os.WriteFile(filepath.Join(staging, "game.exe"), []byte("corrupted"), 0o644); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterCopyBeforeVerify = prev })

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		t.Fatalf("move game: %v", err)
	}
	final := s.wait(job.ID)
	if final.Stage != StageFailed {
		t.Fatalf("stage = %s, want %s", final.Stage, StageFailed)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "game.exe")); err != nil {
		t.Fatalf("source lost after verify failure: %v", err)
	}
	if _, err := os.Stat(target + ".staging"); !os.IsNotExist(err) {
		t.Fatal("staging left behind after verify failure")
	}
	updated, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstallDir != oldDir {
		t.Fatalf("installDir changed despite verify failure: %q", updated.InstallDir)
	}
}

func TestCancelBetweenGames(t *testing.T) {
	set := newTestSettings(t)
	lib := newTestLibrary(t)
	s := newTestService(t, set, lib, nil, nil, nil)

	cfg := set.GetSettings()
	g1 := addGameAt(t, lib, cfg.GamesPath, "Game1")
	g2 := addGameAt(t, lib, cfg.GamesPath, "Game2")

	var once sync.Once
	s.afterItem = func(job Job) {
		once.Do(func() {
			if err := s.Cancel(job.ID); err != nil {
				t.Errorf("cancel: %v", err)
			}
		})
	}

	job, err := s.MoveLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("move library: %v", err)
	}
	final := s.wait(job.ID)
	if final.Stage != StageCancelled {
		t.Fatalf("stage = %s, want %s", final.Stage, StageCancelled)
	}

	moved1, err := lib.Find(g1.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved2, err := lib.Find(g2.ID)
	if err != nil {
		t.Fatal(err)
	}
	movedCount := 0
	if moved1.InstallDir != g1.InstallDir {
		movedCount++
	}
	if moved2.InstallDir != g2.InstallDir {
		movedCount++
	}
	if movedCount != 1 {
		t.Fatalf("expected exactly one game moved before cancel, moved %d of 2", movedCount)
	}
	// Only a completed job is dropped from the journal; a cancelled one
	// stays listed so the UI can still show it.
	if got := s.List(); len(got) == 0 {
		t.Fatal("cancelled library job should stay listed for inspection")
	}
}

// TestConcurrentProgressAndList exercises the concurrency invariant 23
// requires proof for: List() reading the journal while progress ticks and
// stage transitions write it from the move's own goroutine, both guarded by
// the same mutex. Run under -race; it does nothing to catch a race under a
// plain `go test`.
func TestConcurrentProgressAndList(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, oldDir := addTestGame(t, lib)
	for i := 0; i < 40; i++ {
		mustMkdirFile(t, oldDir, "file"+string(rune('a'+i%26))+string(rune('0'+i/26))+".bin", string(make([]byte, 8192)))
	}
	// A missing parent forces the copy/verify path, which reports progress
	// repeatedly instead of jumping straight to done via the rename path.
	target := filepath.Join(t.TempDir(), "missing-parent", "game")

	var reader sync.WaitGroup
	stop := make(chan struct{})
	reader.Add(1)
	go func() {
		defer reader.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.List()
			}
		}
	}()

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		close(stop)
		reader.Wait()
		t.Fatalf("move game: %v", err)
	}
	s.wait(job.ID)
	close(stop)
	reader.Wait()
}

// TestMoveGameRefusesConcurrentSameGame proves the TOCTOU fix in
// registerJob: the busy/target checks and the journal append now share one
// critical section, so a second MoveGame for a game already being moved is
// rejected instead of racing a second copy onto disk. Before the fix,
// registerJob appended unconditionally and both copies would proceed.
func TestMoveGameRefusesConcurrentSameGame(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)
	// A missing parent forces the slower copy/verify path so the first job
	// is still active in s.jobs when the second call arrives.
	targetA := filepath.Join(t.TempDir(), "missing-parent-a", "game")
	targetB := filepath.Join(t.TempDir(), "target-b")

	jobA, err := s.MoveGame(game.ID, targetA)
	if err != nil {
		t.Fatalf("first move: %v", err)
	}
	if _, err := s.MoveGame(game.ID, targetB); !errors.Is(err, ErrMoveInProgress) {
		t.Fatalf("second move err = %v, want ErrMoveInProgress", err)
	}
	s.wait(jobA.ID)

	if _, err := os.Stat(targetB); !os.IsNotExist(err) {
		t.Fatal("second move's target should never have been touched")
	}
	updated, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstallDir != targetA {
		t.Fatalf("installDir = %q, want %q (first move should have won cleanly)", updated.InstallDir, targetA)
	}
}

// TestMoveGameRefusesOverlappingTarget proves the same registerJob fix also
// catches two DIFFERENT games racing onto the same destination: without the
// path-collision check, checkBusy only looks at the gameID, so two unrelated
// games could both be accepted into the same target directory.
func TestMoveGameRefusesOverlappingTarget(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game1, _ := addTestGame(t, lib)
	game2, _ := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "missing-parent", "shared-target")

	job1, err := s.MoveGame(game1.ID, target)
	if err != nil {
		t.Fatalf("first move: %v", err)
	}
	if _, err := s.MoveGame(game2.ID, target); !errors.Is(err, ErrMoveInProgress) {
		t.Fatalf("second move err = %v, want ErrMoveInProgress", err)
	}
	s.wait(job1.ID)

	updated2, err := lib.Find(game2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated2.InstallDir == target {
		t.Fatal("second game should never have been relocated onto the shared target")
	}
}

// TestMoveGameRefusesWhileRunning covers the one checkBusy branch that
// cannot be faked from outside the library package: IsRunning needs an
// actual tracked session, so this launches a real, harmless process and
// kills it via the exported StopGame when done.
func TestMoveGameRefusesWhileRunning(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)

	game, err := lib.AddGame(`C:\Windows\System32\notepad.exe`, "Notepad")
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	if err := lib.PlayGame(game.ID); err != nil {
		t.Fatalf("play game: %v", err)
	}
	t.Cleanup(func() {
		if err := lib.StopGame(game.ID); err != nil {
			t.Logf("stop game: %v", err)
		}
	})

	if _, err := s.MoveGame(game.ID, filepath.Join(t.TempDir(), "target")); !errors.Is(err, ErrGameRunning) {
		t.Fatalf("err = %v, want ErrGameRunning", err)
	}
}

// checkBusy's ErrDownloadBusy branch (s.dl.ByOrigin) is not covered by an
// automated test here. Getting a Download into ByOrigin's results from
// outside the download package requires either AddTask (needs a live
// torrent client fetching real metadata over the network — out of
// proportion for this check) or pre-seeding downloads.json and letting
// ServiceStartup load it — but ServiceStartup's own restore() goroutine
// always wins the race to reattach any non-Completed seeded record to a
// torrent, and with no real .torrent file behind a fake infoHash it always
// loses that reattach and flips the record to StatusFailed (confirmed by
// running it: "restore download ... error=metainfo unavailable", status
// finished within milliseconds) regardless of the seeded status or of
// whether client creation succeeds. There is no exported way to load
// downloads.json without triggering that goroutine. The branch itself is a
// one-line delegation identical in shape to the ErrUpdateBusy/ErrInstallBusy
// cases in TestMoveGameRefusesBusy, which are covered.
