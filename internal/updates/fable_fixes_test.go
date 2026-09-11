package updates

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"typhon/internal/library"
)

func TestAuditFix005(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	next := filepath.Join(root, "new")
	prev := old + ".previous"
	for _, dir := range []string{old, prev} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := lib.RegisterInstalled(library.InstalledGame{Title: "Game", Executable: filepath.Join(old, "game.exe"), InstallDir: old})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.library = lib
	s.rollbacks[g.ID] = &Rollback{GameID: g.ID, Path: prev, InstallDir: old, Executable: filepath.Join(old, "game.exe"), Version: "1"}
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Relocate(g.ID, next); err != nil {
		t.Fatal(err)
	}
	if err := s.Rollback(g.ID); err == nil {
		t.Fatal("rollback at obsolete location accepted")
	}
	after, err := lib.Find(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.InstallDir != next || !exists(next) || exists(old) {
		t.Fatal("old path recreated")
	}

}

type fableSnapshotLibrary struct {
	fakeLibrary
	calls    atomic.Int32
	captured chan struct{}
	release  chan struct{}
}

func (f *fableSnapshotLibrary) GetInstalledGames() []library.Game {
	snap := f.fakeLibrary.GetInstalledGames()
	if f.calls.Add(1) == 1 {
		close(f.captured)
		<-f.release
	}
	return snap
}
func TestAuditFix015(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	lib := &fableSnapshotLibrary{captured: make(chan struct{}), release: make(chan struct{})}
	s.library = lib
	s.releases = &fakeReleases{}
	done := make(chan struct{})
	go func() { s.checkAll(context.Background()); close(done) }()
	<-lib.captured
	lib.mu.Lock()
	lib.games = []library.Game{{ID: "new", Title: "New", InstallDir: t.TempDir()}}
	lib.mu.Unlock()
	secondDone := make(chan struct{})
	go func() { s.checkAll(context.Background()); close(secondDone) }()
	close(lib.release)
	<-done
	<-secondDone
	_, after := s.snapshot("new")
	if !after {
		t.Fatal("concurrent check removed new update record")
	}

}

func TestAuditFix011(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	previous := current + ".previous"
	for _, dir := range []string{current, previous} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.swapDirectories("g1", current, filepath.Join(root, "missing-stage"), previous, "3")
	if err == nil || !exists(current) || !exists(previous) {
		t.Fatal("011 reproduced: failed second update loses previous rollback copy")
	}
}
