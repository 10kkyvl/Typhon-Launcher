package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditFix003(t *testing.T) {
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := s.RegisterInstalled(InstalledGame{Title: "Game", Executable: exe, InstallDir: dir, ReleaseID: "r1", SourceID: "s1", DistributionID: "d1", Repacker: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkUninstalled(g.ID); err != nil {
		t.Fatal(err)
	}
	g, err = s.RegisterInstalled(InstalledGame{Title: "Game", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if g.ReleaseID != "" || g.DistributionID != "" || g.Repacker != "" {
		t.Fatalf("003 reproduced: release=%s distribution=%s repacker=%s", g.ReleaseID, g.DistributionID, g.Repacker)
	}
}

func TestAuditFix007(t *testing.T) {
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGame(exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	held := false
	s.ctx = context.Background()
	s.prepare = func(context.Context, launch) error {
		held = !s.mu.TryLock()
		if !held {
			s.mu.Unlock()
		}
		return errors.New("stop before real launch")
	}
	_ = s.PlayGame(g.ID)
	if held {
		t.Fatal("007 reproduced: global library mutex held during runtime preparation")
	}
}

func TestAuditFix008MeasuresOutsideLibraryLock(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	old := measureInstall
	defer func() { measureInstall = old }()
	calls := 0
	measureInstall = func(id, dir string) (int64, bool) {
		calls++
		if !s.mu.TryLock() {
			t.Fatal("measurement holds library mutex")
		}
		s.mu.Unlock()
		return old(id, dir)
	}
	g, err := s.RegisterInstalled(InstalledGame{Title: "Game", InstallDir: dir, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterInstalled(InstalledGame{Title: "Game", InstallDir: dir, Executable: exe}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyInstalledUpdate(InstalledUpdate{ID: g.ID, Version: "2"}); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("measured %d times", calls)
	}
}

func TestAuditFix001OldCloudOwnershipIsNotRestored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGame(exe, "Manual")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.games[0].Owned = true
	err = s.persist()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := reopened.Find(g.ID)
	if err != nil || after.Owned {
		t.Fatalf("cloud ownership retained: %+v %v", after, err)
	}
}

type auditProcess struct{ done chan struct{} }

func (p *auditProcess) pid() int    { return 1234 }
func (p *auditProcess) wait() error { <-p.done; return nil }
func (p *auditProcess) kill() error { close(p.done); return nil }

func TestAuditFix007LaunchContextOutlivesCall(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGame(exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	var launchCtx context.Context
	s.start = func(ctx context.Context, _ launch) (gameProcess, error) {
		launchCtx = ctx
		return &auditProcess{done: make(chan struct{})}, nil
	}
	if err := s.PlayGame(g.ID); err != nil {
		t.Fatal(err)
	}
	if launchCtx.Err() != nil {
		t.Fatal("launch return canceled process monitoring")
	}
	if err := s.StopGame(g.ID); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()
	if launchCtx.Err() == nil {
		t.Fatal("process completion leaked launch context")
	}
}

func TestAuditFix007PreparingRemainsResponsiveAndReserved(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGame(exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	done := make(chan error, 1)
	s.prepare = func(ctx context.Context, _ launch) error { close(entered); <-ctx.Done(); return ctx.Err() }
	go func() { done <- s.PlayGame(g.ID) }()
	<-entered
	if _, err := s.Find(g.ID); err != nil {
		t.Fatal(err)
	}
	if !s.IsRunning(g.ID) {
		t.Fatal("starting game was not reserved")
	}
	if err := s.PlayGame(g.ID); err == nil {
		t.Fatal("duplicate launch accepted")
	}
	if err := s.StopGame(g.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if s.IsRunning(g.ID) {
		t.Fatal("canceled launch kept reservation")
	}
}
