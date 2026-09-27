package relocate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditFix004Cleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "game.exe"), []byte("game"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServiceAt(t.TempDir(), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.jobs = []Job{{ID: "job", Source: source, Target: target, Stage: StagePrepare}}
	afterCopyBeforeVerify = func(string) {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "collision"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { afterCopyBeforeVerify = nil }()
	err = s.copyAndVerify(context.Background(), "job", source, target)
	s.recoverAll(context.Background())
	if err == nil || exists(target+".staging") || !exists(source) {
		t.Fatal("004 cleanup reproduced: failed commit rename leaves full staging even after recovery; Windows rename not executed")
	}
}

type rollbackKeeper struct{}

func (rollbackKeeper) Busy(string) bool        { return false }
func (rollbackKeeper) HasRollback(string) bool { return true }
func TestAuditFix005MoveRefusesRetainedRollback(t *testing.T) {
	lib := newTestLibrary(t)
	game, dir := addTestGame(t, lib)
	s := newTestService(t, nil, lib, nil, nil, rollbackKeeper{})
	if _, err := s.MoveGame(game.ID, filepath.Join(t.TempDir(), "new")); err == nil {
		t.Fatal("move with rollback accepted")
	}
	if !exists(dir) {
		t.Fatal("refused move changed source")
	}
}

func TestAuditFix004MoveIntoExistingEmptyDirectory(t *testing.T) {
	lib := newTestLibrary(t)
	s := newTestService(t, nil, lib, nil, nil, nil)
	g, old := addTestGame(t, lib)
	target := filepath.Join(filepath.Dir(old), "existing")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	job, err := s.MoveGame(g.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	s.wait(job.ID)
	after, err := lib.Find(g.ID)
	if err != nil || after.InstallDir != target {
		t.Fatalf("move: %+v %v", after, err)
	}
	if exists(old) || exists(target+".staging") || !exists(filepath.Join(target, "game.exe")) {
		t.Fatal("move left incorrect filesystem state")
	}
}
