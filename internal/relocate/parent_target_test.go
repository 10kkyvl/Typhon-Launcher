package relocate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"typhon/internal/hashdir"
)

func TestMoveGameIntoNonEmptyParent(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := t.TempDir()
	mustMkdirFile(t, filepath.Join(parent, "OtherGame"), "other.exe", "other")
	want := filepath.Join(parent, filepath.Base(src))

	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatalf("move game: %v", err)
	}
	s.wait(job.ID)
	updated, err := lib.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstallDir != want {
		t.Fatalf("installDir = %q, want %q", updated.InstallDir, want)
	}
	if !exists(filepath.Join(want, "game.exe")) || !exists(filepath.Join(parent, "OtherGame", "other.exe")) {
		t.Fatal("game or the neighbour is missing after the move")
	}
	if exists(src) {
		t.Fatal("old folder survived")
	}
}

func TestMoveGameTargetFolderIsOccupied(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := t.TempDir()
	occupied := mustMkdirFile(t, filepath.Join(parent, filepath.Base(src)), "keep.txt", "mine")

	if _, err := s.MoveGame(game.ID, parent); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	}
	body, err := os.ReadFile(occupied)
	if err != nil || string(body) != "mine" {
		t.Fatalf("occupied folder touched: %q %v", body, err)
	}
	if !exists(filepath.Join(src, "game.exe")) {
		t.Fatal("source touched")
	}
}

func TestMoveGameParentErrors(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	nested := addGameAt(t, lib, filepath.Join(t.TempDir(), "Inner"), "Inner")
	nestedParent := filepath.Dir(filepath.Dir(nested.InstallDir))

	cases := []struct {
		name   string
		gameID string
		parent string
		want   error
	}{
		{"empty", game.ID, "", ErrEmptyTarget},
		{"blank", game.ID, "   ", ErrEmptyTarget},
		{"relative", game.ID, "relative/path", ErrRelativeTarget},
		{"same location", game.ID, filepath.Dir(src), ErrSameLocation},
		{"inside source", game.ID, src, ErrTargetInsideSource},
		{"source inside target", nested.ID, nestedParent, ErrSourceInsideTarget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.MoveGame(tc.gameID, tc.parent); !errors.Is(err, tc.want) {
				t.Fatalf("MoveGame err = %v, want %v", err, tc.want)
			}
			if _, err := s.MoveGameTarget(tc.gameID, tc.parent); !errors.Is(err, tc.want) {
				t.Fatalf("MoveGameTarget err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMoveGameTargetMatchesMoveGame(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	game, src := addTestGame(t, lib)
	parent := filepath.Join(t.TempDir(), "not", "created", "yet")

	got, err := s.MoveGameTarget(game.ID, parent)
	if err != nil {
		t.Fatalf("MoveGameTarget: %v", err)
	}
	if want := filepath.Join(parent, filepath.Base(src)); got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if exists(parent) {
		t.Fatal("MoveGameTarget created folders")
	}
	job, err := s.MoveGame(game.ID, parent)
	if err != nil {
		t.Fatalf("MoveGame: %v", err)
	}
	if job.Target != got {
		t.Fatalf("job target = %q, MoveGameTarget said %q", job.Target, got)
	}
	s.wait(job.ID)
	updated, err := lib.Find(game.ID)
	if err != nil || updated.InstallDir != got {
		t.Fatalf("installDir = %q (%v), want %q", updated.InstallDir, err, got)
	}
}

func TestMoveGameTargetAcceptsDriveRootAsParent(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	dir := t.TempDir()
	name := "typhon-root-" + filepath.Base(filepath.Dir(dir))
	game := addGameAt(t, lib, dir, name)
	src := game.InstallDir
	root := filepath.VolumeName(src) + string(filepath.Separator)
	if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("setup: %s exists or is unreadable: %v", filepath.Join(root, name), err)
	}

	got, err := s.MoveGameTarget(game.ID, root)
	if err != nil {
		t.Fatalf("MoveGameTarget: %v", err)
	}
	if want := filepath.Join(root, filepath.Base(src)); got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRecoveryLogsOneLinePerJob(t *testing.T) {
	var logs lockedBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	root := t.TempDir()
	src := filepath.Join(root, "src")
	mustMkdirFile(t, src, "game.exe", "data")
	copying := Job{ID: "j-copy", Scope: ScopeGame, Stage: StageCopy, GameID: itemScreenshots, Source: src, Target: filepath.Join(root, "tgt"), TotalBytes: 4}
	done := Job{ID: "j-done", Scope: ScopeGame, Stage: StageDone, GameID: itemScreenshots, Source: src, Target: filepath.Join(root, "tgt2")}
	s := newRecoverTestService(t, copying)
	s.jobs = []Job{copying, done}
	if err := s.st.saveJournal(s.jobs); err != nil {
		t.Fatal(err)
	}
	if m, err := hashdir.Build(context.Background(), src, nil); err != nil {
		t.Fatal(err)
	} else if err := s.st.saveManifest(copying.ID, m); err != nil {
		t.Fatal(err)
	}

	s.recoverAll(context.Background())

	var copyLines, doneLines []string
	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, "move recovery finished") {
			continue
		}
		switch {
		case strings.Contains(line, "job=j-copy"):
			copyLines = append(copyLines, line)
		case strings.Contains(line, "job=j-done"):
			doneLines = append(doneLines, line)
		}
	}
	if len(copyLines) != 1 || len(doneLines) != 1 {
		t.Fatalf("lines: copy %d, done %d, want one each\n%s", len(copyLines), len(doneLines), logs.String())
	}
	for _, want := range []string{"level=WARN", "found_stage=copy", "outcome=failed", "reason=", "game=@screenshots"} {
		if !strings.Contains(copyLines[0], want) {
			t.Errorf("failed job line lacks %q: %s", want, copyLines[0])
		}
	}
	for _, want := range []string{"level=INFO", "found_stage=done", "outcome=surfaced"} {
		if !strings.Contains(doneLines[0], want) {
			t.Errorf("surfaced job line lacks %q: %s", want, doneLines[0])
		}
	}
}
