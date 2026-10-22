package relocate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/library"
)

func TestRepointFailureFailsTheJob(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")

	afterCopyBeforeVerify = func(string) {
		if err := lib.RemoveGame(game.ID); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterCopyBeforeVerify = nil })

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		t.Fatalf("move game: %v", err)
	}
	final := s.wait(job.ID)
	if final.Stage != StageFailed {
		t.Fatalf("stage = %s, want %s", final.Stage, StageFailed)
	}
	if final.Error == "" {
		t.Fatal("failed job carries no error")
	}
	if final.FailedAt != StageRepoint {
		t.Fatalf("failedAt = %q, want %q", final.FailedAt, StageRepoint)
	}
	if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
		t.Fatalf("source lost: %v", err)
	}
	journal, err := s.st.loadJournal()
	if err != nil {
		t.Fatal(err)
	}
	if len(journal) != 1 || journal[0].Stage != StageFailed {
		t.Fatalf("journal = %+v, want one failed job", journal)
	}
	if err := s.Cancel(job.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("cancel of a finished job = %v, want ErrJobNotFound", err)
	}
}

func TestCleanupFailureIsRetriedOnStartup(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")

	removeTree = func(string) error { return syscall.EACCES }
	t.Cleanup(func() { removeTree = os.RemoveAll })

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		t.Fatalf("move game: %v", err)
	}
	final := s.wait(job.ID)
	if final.Stage != StageFailed || final.FailedAt != StageCleanup {
		t.Fatalf("stage = %s failedAt = %q, want failed at %s", final.Stage, final.FailedAt, StageCleanup)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source must stay after a failed cleanup: %v", err)
	}

	removeTree = os.RemoveAll
	s.recoverAll(context.Background())

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("old copy survived startup recovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, filepath.Base(src), "game.exe")); err != nil {
		t.Fatalf("new copy lost: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job should be gone after the retry: %+v", got)
	}
}

func TestRecoverFailedJobAfterRepoint(t *testing.T) {
	cases := []struct {
		name         string
		failedAt     Stage
		libraryAtNew bool
		corruptNew   bool
		wantSrcGone  bool
		wantAtNew    bool
		wantJobGone  bool
	}{
		{"cleanup failed, new copy registered", StageCleanup, true, false, true, true, true},
		{"cleanup failed, library still at old copy", StageCleanup, false, false, false, false, true},
		{"cleanup failed, new copy corrupt", StageCleanup, true, true, false, true, false},
		{"repoint failed, library still at old copy", StageRepoint, false, false, true, true, true},
		{"repoint failed, new copy corrupt", StageRepoint, false, true, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lib := newTestLibrary(t)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "target")
			mustMkdirFile(t, target, "game.exe", "stub")
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.libraryAtNew {
				if _, err := lib.Relocate(game.ID, target); err != nil {
					t.Fatal(err)
				}
				manifest, err = hashdir.Build(context.Background(), target, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tc.corruptNew {
				mustMkdirFile(t, target, "game.exe", "corrupt")
			}
			job := Job{
				ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: tc.failedAt, Error: "boom",
				GameID: game.ID, Source: src, Target: target,
			}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}

			s.recoverJob(context.Background(), job)

			if got := !exists(src); got != tc.wantSrcGone {
				t.Fatalf("source gone = %v, want %v", got, tc.wantSrcGone)
			}
			stored, err := lib.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := stored.InstallDir == target; got != tc.wantAtNew {
				t.Fatalf("library at new copy = %v, want %v", got, tc.wantAtNew)
			}
			if got := len(s.List()) == 0; got != tc.wantJobGone {
				t.Fatalf("job gone = %v, want %v (%+v)", got, tc.wantJobGone, s.List())
			}
		})
	}
}

func registerOutsideGame(t *testing.T, lib *library.Service) (library.Game, string) {
	t.Helper()
	installDir := filepath.Join(t.TempDir(), "install")
	mustMkdirFile(t, installDir, "data.bin", "data")
	exe := mustMkdirFile(t, filepath.Join(t.TempDir(), "elsewhere"), "game.exe", "stub")
	game, err := lib.RegisterInstalled(t.Context(), library.InstalledGame{Title: "Outside", Executable: exe, InstallDir: installDir})
	if err != nil {
		t.Fatal(err)
	}
	return game, installDir
}

func TestMoveGameRefusesExecutableOutsideBeforeMoving(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, installDir := registerOutsideGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")

	if _, err := s.MoveGame(game.ID, target); !errors.Is(err, library.ErrExecutableOutside) {
		t.Fatalf("err = %v, want ErrExecutableOutside", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "data.bin")); err != nil {
		t.Fatalf("source touched: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target created: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job registered: %+v", got)
	}
}

func TestMoveLibraryRefusesExecutableOutsideBeforeMoving(t *testing.T) {
	set := newTestSettings(t)
	lib := newTestLibrary(t)
	s := newTestService(t, set, lib, nil, nil, nil)
	cfg := set.GetSettings()
	good := addGameAt(t, lib, cfg.GamesPath, "Good")
	registerOutsideGame(t, lib)

	if _, err := s.MoveLibrary(t.TempDir()); !errors.Is(err, library.ErrExecutableOutside) {
		t.Fatalf("err = %v, want ErrExecutableOutside", err)
	}
	if _, err := os.Stat(good.InstallDir); err != nil {
		t.Fatalf("a game was moved despite the refusal: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job registered: %+v", got)
	}
}

func TestRenameErrorOtherThanCrossDeviceFailsTheJob(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCopy bool
	}{
		{"access denied", syscall.EACCES, false},
		{"missing path", syscall.ENOENT, false},
		{"cross device", errCrossDevice, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := renameDir
			renameDir = func(oldpath, newpath string) error {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: tc.err}
			}
			t.Cleanup(func() { renameDir = prev })
			lib := newTestLibrary(t)
			s := newTestService(t, nil, lib, nil, nil, nil)
			game, src := addTestGame(t, lib)
			target := filepath.Join(t.TempDir(), "target")

			job, err := s.MoveGame(game.ID, target)
			if err != nil {
				t.Fatal(err)
			}
			final := s.wait(job.ID)
			if tc.wantCopy {
				if _, err := os.Stat(filepath.Join(target, filepath.Base(src), "game.exe")); err != nil {
					t.Fatalf("copy fallback did not run: %v", err)
				}
				return
			}
			if final.Stage != StageFailed {
				t.Fatalf("stage = %q, want %s", final.Stage, StageFailed)
			}
			if !strings.Contains(final.Error, tc.err.Error()) {
				t.Fatalf("error %q does not carry the rename cause %q", final.Error, tc.err)
			}
			if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
				t.Fatalf("source lost: %v", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("target created by a failed rename: %v", err)
			}
			if _, err := os.Stat(target + ".staging"); !os.IsNotExist(err) {
				t.Fatalf("copy fallback ran: %v", err)
			}
		})
	}
}

func TestLaunchIsRefusedWhileTheGameIsMoving(t *testing.T) {
	forceCopyPath(t)
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")

	var playErr error
	afterCopyBeforeVerify = func(string) { playErr = lib.PlayGame(game.ID) }
	t.Cleanup(func() { afterCopyBeforeVerify = nil })

	job, err := s.MoveGame(game.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	s.wait(job.ID)
	if !errors.Is(playErr, library.ErrGameRelocating) {
		t.Fatalf("PlayGame during the move = %v, want ErrGameRelocating", playErr)
	}
	if err := lib.PlayGame(game.ID); errors.Is(err, library.ErrGameRelocating) {
		t.Fatal("the launch lock outlived the job")
	}
}

func TestMoveGameRefusesAGameLockedForAMove(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)

	release, err := lib.BeginRelocate(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if _, err := s.MoveGame(game.ID, filepath.Join(t.TempDir(), "target")); err == nil {
		t.Fatal("MoveGame accepted a game that is already locked for a move")
	}
	if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
		t.Fatalf("source touched: %v", err)
	}
}

func TestVerifyManifestIgnoresTheInstallMarker(t *testing.T) {
	cases := []struct {
		name      string
		sourceHas bool
		targetHas bool
		corrupt   bool
		wantErr   bool
	}{
		{"marker only in target", false, true, false, false},
		{"marker rewritten in target", true, true, false, false},
		{"marker dropped from target", true, false, false, false},
		{"other file corrupt", false, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "src")
			tgt := filepath.Join(root, "tgt")
			mustMkdirFile(t, src, "game.exe", "stub")
			mustMkdirFile(t, tgt, "game.exe", "stub")
			if tc.sourceHas {
				mustMkdirFile(t, src, library.MarkerName, "old")
			}
			if tc.targetHas {
				mustMkdirFile(t, tgt, library.MarkerName, "new")
			}
			if tc.corrupt {
				mustMkdirFile(t, tgt, "game.exe", "bad")
			}
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := checkManifest(context.Background(), tgt, manifest)
			if err == nil && hasIssues(result) {
				err = ErrVerifyFailed
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
