package relocate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fsys := os.DirFS(root)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		out[filepath.FromSlash(path)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRecoverRepointKeepsAChangedSource(t *testing.T) {
	cases := []struct {
		name         string
		item         string
		withManifest bool
		wantTarget   bool
		wantTerminal bool
	}{
		{"game", "", true, false, true},
		{"screenshots", itemScreenshots, true, false, true},
		{"downloads without a manifest", itemDownloads, false, true, true},
		{"settings", itemSettings, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "src")
			target := filepath.Join(root, "target")
			var lib *library.Service
			gameID := tc.item
			if tc.item == "" {
				lib = newTestLibrary(t)
				game, dir := addTestGame(t, lib)
				src = dir
				gameID = game.ID
			} else {
				mustMkdirFile(t, src, "data.bin", "original")
			}
			if tc.item == "" {
				mustMkdirFile(t, target, "game.exe", "stub")
			} else {
				mustMkdirFile(t, target, "data.bin", "original")
			}
			manifest, err := hashdir.Build(context.Background(), src, nil)
			if err != nil {
				t.Fatal(err)
			}
			mustMkdirFile(t, src, "save.dat", "fresh progress")

			job := Job{
				ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: StageRepoint, Error: "boom",
				GameID: gameID, Source: src, Target: target,
			}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if tc.withManifest {
				if err := s.st.saveManifest(job.ID, manifest); err != nil {
					t.Fatal(err)
				}
			}
			if tc.item == itemSettings {
				set := newTestSettings(t)
				cfg := set.GetSettings()
				cfg.LibraryPath = src
				if err := set.SaveSettings(cfg); err != nil {
					t.Fatal(err)
				}
				s.settings = set
			}
			before := snapshotTree(t, src)
			targetBefore := snapshotTree(t, target)

			s.recoverJob(context.Background(), job)

			if after := snapshotTree(t, src); !reflect.DeepEqual(before, after) {
				t.Fatalf("source changed by recovery:\nbefore %v\nafter  %v", before, after)
			}
			if got := exists(target); got != tc.wantTarget {
				t.Fatalf("target exists = %v, want %v", got, tc.wantTarget)
			}
			if tc.wantTarget && !reflect.DeepEqual(targetBefore, snapshotTree(t, target)) {
				t.Fatal("target changed although it could not be proven a pure copy")
			}
			if tc.item == "" {
				stored, err := lib.Find(gameID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.InstallDir != src {
					t.Fatalf("library moved off the live copy: %q", stored.InstallDir)
				}
			}
			if tc.wantTerminal {
				got := s.soleJob(t)
				if got.Stage != StageFailed || got.FailedAt != "" {
					t.Fatalf("job = %s/%q, want a terminal failed job", got.Stage, got.FailedAt)
				}
			}
		})
	}
}

func TestRecoveryReservesTheGame(t *testing.T) {
	cases := []struct {
		name     string
		failedAt Stage
		corrupt  bool
	}{
		{"cleanup retry", StageCleanup, false},
		{"repoint retry", StageRepoint, false},
		{"source changed", StageRepoint, true},
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
			if tc.failedAt == StageCleanup {
				if _, err := lib.Relocate(game.ID, target); err != nil {
					t.Fatal(err)
				}
				manifest, err = hashdir.Build(context.Background(), target, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tc.corrupt {
				mustMkdirFile(t, src, "save.dat", "fresh")
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: tc.failedAt, Error: "boom", GameID: game.ID, Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}
			var playErr error
			removeTree = func(path string) error {
				playErr = lib.PlayGame(game.ID)
				return os.RemoveAll(path)
			}
			t.Cleanup(func() { removeTree = os.RemoveAll })
			afterCopyBeforeVerify = nil

			s.recoverJob(context.Background(), job)

			if !tc.corrupt && !errors.Is(playErr, library.ErrGameRelocating) {
				t.Fatalf("PlayGame during recovery = %v, want ErrGameRelocating", playErr)
			}
			release, err := lib.BeginRelocate(game.ID)
			if err != nil {
				t.Fatalf("reservation leaked past recovery: %v", err)
			}
			release()
		})
	}
}

func TestRecoveryStopsAfterThreeAttempts(t *testing.T) {
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")
	mustMkdirFile(t, target, "game.exe", "stub")
	if err := os.MkdirAll(filepath.Join(target, library.MarkerName), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageFailed, FailedAt: StageRepoint, Error: "boom", GameID: game.ID, Source: src, Target: target}
	s := newRecoverTestService(t, job)
	s.lib = lib
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, src)

	for attempt := 1; attempt <= 2; attempt++ {
		s.recoverJob(context.Background(), s.soleJob(t))
		got := s.soleJob(t)
		if got.Stage != StageFailed || got.FailedAt != StageRepoint || got.RecoveryAttempts != attempt {
			t.Fatalf("attempt %d: job = %s/%q attempts=%d, want failed/repoint with %d attempts", attempt, got.Stage, got.FailedAt, got.RecoveryAttempts, attempt)
		}
	}
	s.recoverJob(context.Background(), s.soleJob(t))

	got := s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != "" {
		t.Fatalf("job = %s/%q after the last attempt, want a terminal failed job", got.Stage, got.FailedAt)
	}
	if !strings.Contains(got.Error, "relocate.retries_exhausted") || !strings.Contains(got.Error, "boom") {
		t.Fatalf("error %q must carry the exhausted code and the last error", got.Error)
	}
	if !reflect.DeepEqual(before, snapshotTree(t, src)) {
		t.Fatal("source changed while retrying")
	}
}

func TestRecoveryOfARemovedGameIsTerminalAtOnce(t *testing.T) {
	cases := []struct {
		name     string
		failedAt Stage
		stage    Stage
	}{
		{"failed repoint", StageRepoint, StageFailed},
		{"failed cleanup", StageCleanup, StageFailed},
		{"crashed at repoint", "", StageRepoint},
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
			if err := lib.RemoveGame(game.ID); err != nil {
				t.Fatal(err)
			}
			job := Job{ID: "j1", Scope: ScopeGame, Stage: tc.stage, FailedAt: tc.failedAt, Error: "boom", GameID: game.ID, Source: src, Target: target}
			s := newRecoverTestService(t, job)
			s.lib = lib
			if err := s.st.saveManifest(job.ID, manifest); err != nil {
				t.Fatal(err)
			}
			srcBefore, targetBefore := snapshotTree(t, src), snapshotTree(t, target)

			s.recoverJob(context.Background(), job)

			got := s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != "" || got.RecoveryAttempts != 0 {
				t.Fatalf("job = %s/%q attempts=%d, want terminal at once", got.Stage, got.FailedAt, got.RecoveryAttempts)
			}
			if !strings.Contains(got.Error, "relocate.game_removed") {
				t.Fatalf("error %q does not carry relocate.game_removed", got.Error)
			}
			if !reflect.DeepEqual(srcBefore, snapshotTree(t, src)) || !reflect.DeepEqual(targetBefore, snapshotTree(t, target)) {
				t.Fatal("recovery deleted data of a removed game")
			}
		})
	}
}

func TestUserVisibleErrorsCarryCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"executable outside", library.ErrExecutableOutside, "library.executable_outside"},
		{"relocating", library.ErrGameRelocating, "library.relocating"},
		{"source changed", ErrSourceChanged, "relocate.source_changed"},
		{"source unverifiable", ErrSourceUnverifiable, "relocate.source_unverifiable"},
		{"game removed", ErrGameRemoved, "relocate.game_removed"},
		{"retries exhausted", ErrRetriesExhausted, "relocate.retries_exhausted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uierr.Code(tc.err); got != tc.want {
				t.Fatalf("code = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailedMoveRemovesTheParentsItCreated(t *testing.T) {
	cases := []struct {
		name       string
		renameErr  error
		corruptNew bool
	}{
		{"rename fails", errors.New("rename refused"), false},
		{"copy fails verification", errCrossDevice, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := renameDir
			renameDir = func(oldpath, newpath string) error {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: tc.renameErr}
			}
			t.Cleanup(func() { renameDir = prev })
			if tc.corruptNew {
				afterCopyBeforeVerify = func(staging string) {
					if err := os.WriteFile(filepath.Join(staging, "game.exe"), []byte("corrupted"), 0o644); err != nil {
						t.Error(err)
					}
				}
				t.Cleanup(func() { afterCopyBeforeVerify = nil })
			}
			lib := newTestLibrary(t)
			s := newTestService(t, nil, lib, nil, nil, nil)
			game, _ := addTestGame(t, lib)
			existing := t.TempDir()
			target := filepath.Join(existing, "a", "b", "target")

			job, err := s.MoveGame(game.ID, target)
			if err != nil {
				t.Fatal(err)
			}
			if final := s.wait(job.ID); final.Stage != StageFailed {
				t.Fatalf("stage = %s, want failed", final.Stage)
			}
			if _, err := os.Stat(filepath.Join(existing, "a")); !os.IsNotExist(err) {
				t.Fatalf("directories created for the failed move were left behind: %v", err)
			}
			if _, err := os.Stat(existing); err != nil {
				t.Fatalf("pre-existing directory removed: %v", err)
			}
		})
	}
}

func TestFailedMoveKeepsAnExistingParent(t *testing.T) {
	prev := renameDir
	renameDir = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("rename refused")}
	}
	t.Cleanup(func() { renameDir = prev })
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, _ := addTestGame(t, lib)
	existing := t.TempDir()
	mustMkdirFile(t, existing, "keep.txt", "x")

	job, err := s.MoveGame(game.ID, filepath.Join(existing, "target"))
	if err != nil {
		t.Fatal(err)
	}
	s.wait(job.ID)
	if _, err := os.Stat(filepath.Join(existing, "keep.txt")); err != nil {
		t.Fatalf("existing parent damaged: %v", err)
	}
}
