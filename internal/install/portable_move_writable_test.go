package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/storage"
)

func markReadOnly(t *testing.T, files ...string) {
	t.Helper()
	for _, file := range files {
		if err := os.Chmod(file, 0o400); err != nil {
			t.Fatalf("mark %s read-only: %v", file, err)
		}
	}
}

func assertGameWritable(t *testing.T, dest string, want bool) {
	t.Helper()
	for _, file := range []string{filepath.Join(dest, "Game.exe"), filepath.Join(dest, "data", "content.pak")} {
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0)
		if err == nil {
			if cerr := f.Close(); cerr != nil {
				t.Fatalf("close %s: %v", file, cerr)
			}
		}
		if (err == nil) != want {
			t.Fatalf("open %s for writing: %v, want writable: %v", file, err, want)
		}
	}
}

func useClearMovedReadOnly(t *testing.T, fn func(context.Context, string) error) {
	t.Helper()
	previous := clearMovedReadOnly
	clearMovedReadOnly = fn
	t.Cleanup(func() { clearMovedReadOnly = previous })
}

type movedGame struct {
	r      *rig
	source string
	dest   string
}

func readOnlyMove(t *testing.T) movedGame {
	t.Helper()
	r := newRig(t)
	r.s.sameVolume = sameVolumes
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	markReadOnly(t, filepath.Join(source, "Game.exe"), filepath.Join(source, "data", "content.pak"))
	r.download("d1", "Game", root)
	return movedGame{r: r, source: source, dest: filepath.Join(t.TempDir(), "Games", "Game")}
}

func TestPortableMoveLeavesTheGameWritable(t *testing.T) {
	cases := []struct {
		name    string
		volumes func(a, b string) bool
	}{
		{"renamed on the same volume", sameVolumes},
		{"copied across volumes", differentVolumes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := readOnlyMove(t)
			m.r.s.sameVolume = tc.volumes

			item, err := m.r.s.Start("d1", StartOptions{Destination: m.dest, Mode: ModeMove})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := m.r.settle(item.ID)
			if got.Status != StatusCompleted {
				t.Fatalf("status = %s (%q), want completed", got.Status, got.Error)
			}
			assertGameWritable(t, m.dest, true)
		})
	}
}

func TestPortableMoveSymlinkTargetStaysReadOnly(t *testing.T) {
	m := readOnlyMove(t)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	mkFile(t, outside, 16)
	markReadOnly(t, outside)
	if err := makeDirLink(filepath.Join(m.source, "link"), outsideDir); err != nil {
		t.Fatalf("link out of the game folder: %v", err)
	}

	item, err := m.r.s.Start("d1", StartOptions{Destination: m.dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := m.r.settle(item.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("status = %s (%q), want completed", got.Status, got.Error)
	}
	assertGameWritable(t, m.dest, true)
	info, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatal("the walk followed a symlink out of the game folder")
	}
}

func TestPortableMoveFailsWhenFilesStayReadOnly(t *testing.T) {
	cause := errors.New("access is denied")
	m := readOnlyMove(t)
	useClearMovedReadOnly(t, func(context.Context, string) error { return cause })

	item, err := m.r.s.Start("d1", StartOptions{Destination: m.dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	failed := m.r.settle(item.ID)
	if failed.Status != StatusFailed || !strings.Contains(failed.Error, errMovedReadOnly.Error()) || !strings.Contains(failed.Error, cause.Error()) {
		t.Fatalf("status = %s (%q), want failed with the cause", failed.Status, failed.Error)
	}
	if games := m.r.reg.registered(); len(games) != 0 {
		t.Fatalf("registered games = %+v, want none: the game cannot write its files", games)
	}
	if ids := m.r.downloads.deletedIDs(); len(ids) != 0 {
		t.Fatalf("download records deleted = %v, want none before the install is done", ids)
	}
	if exists(m.source) || !exists(filepath.Join(m.dest, "Game.exe")) {
		t.Fatal("the rename must stay in place: the folder is the only copy")
	}

	useClearMovedReadOnly(t, storage.MakeWritable)
	if err := m.r.s.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	done := m.r.settle(item.ID)
	if done.Status != StatusCompleted || done.GameID == "" {
		t.Fatalf("after retry: %+v (%q)", viewOf(done), done.Error)
	}
	assertGameWritable(t, m.dest, true)
}

func TestPortableMoveCancelDuringTheWalkKeepsTheOnlyCopy(t *testing.T) {
	m := readOnlyMove(t)
	cancelInstall := func() error {
		items := m.r.s.List()
		if len(items) != 1 {
			return errors.New("the install record is missing")
		}
		return m.r.s.Cancel(items[0].ID)
	}
	useClearMovedReadOnly(t, func(ctx context.Context, dir string) error {
		if err := cancelInstall(); err != nil {
			return err
		}
		return storage.MakeWritable(ctx, dir)
	})

	item, err := m.r.s.Start("d1", StartOptions{Destination: m.dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancelled := m.r.settle(item.ID)
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s (%q), want cancelled", cancelled.Status, cancelled.Error)
	}
	if games := m.r.reg.registered(); len(games) != 0 {
		t.Fatalf("registered games = %+v, want none", games)
	}
	m.r.s.wg.Wait()
	if !exists(filepath.Join(m.dest, "Game.exe")) {
		t.Fatal("the cancel removed the only copy of the game")
	}
	assertGameWritable(t, m.dest, false)

	useClearMovedReadOnly(t, storage.MakeWritable)
	if err := m.r.s.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if done := m.r.settle(item.ID); done.Status != StatusCompleted {
		t.Fatalf("after retry: %+v (%q)", viewOf(done), done.Error)
	}
	assertGameWritable(t, m.dest, true)
}

func TestPortableMoveCrashRecoveryClearsReadOnly(t *testing.T) {
	cases := []struct {
		name       string
		partly     bool
		clearErr   error
		wantStatus Status
		wantGame   bool
	}{
		{"crashed right after the rename", false, nil, StatusCompleted, true},
		{"crashed in the middle of the walk", true, nil, StatusCompleted, true},
		{"walk fails after the restart", false, errors.New("access is denied"), StatusFailed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := readOnlyMove(t)
			if err := os.MkdirAll(filepath.Dir(m.dest), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(m.source, m.dest); err != nil {
				t.Fatalf("simulate the rename: %v", err)
			}
			if tc.partly {
				if err := os.Chmod(filepath.Join(m.dest, "Game.exe"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			m.r.shutdown()
			crashed := Installation{
				ID: "crash1", DownloadID: "d1", Name: "Game", Type: TypePortable, Status: StatusVerifying,
				Mode: ModeMove, SourcePath: m.source, ContentRoot: m.source, Destination: m.dest, StartedAt: time.Now(),
			}
			if err := newStore(m.r.dir).save([]Installation{crashed}); err != nil {
				t.Fatalf("write the state of the crashed install: %v", err)
			}
			if tc.clearErr != nil {
				useClearMovedReadOnly(t, func(context.Context, string) error { return tc.clearErr })
			}
			m.r.boot()

			got := m.r.settle("crash1")
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, tc.wantStatus)
			}
			if registered := m.r.reg.registered(); (len(registered) == 1) != tc.wantGame {
				t.Fatalf("registered games = %+v, want a game: %v", registered, tc.wantGame)
			}
			if tc.wantGame {
				assertGameWritable(t, m.dest, true)
			}
		})
	}
}
