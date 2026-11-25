package install

import (
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
)

func (f *fakeRegistrar) setOnRemove(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onRemove = hook
}

// startBlockedInstall starts an install of a download with an installer that
// runs until release is closed, and returns once the installer has begun.
func startBlockedInstall(t *testing.T, s *Service, downloads *fakeDownloads) (Installation, chan struct{}) {
	t.Helper()
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "setup.exe"), 4096)
	downloads.add("d1", "Game", root)
	release := make(chan struct{})
	entered := make(chan struct{})
	s.roots = []string{t.TempDir()}
	s.runner = &fakeRunner{act: func(runSpec) {
		close(entered)
		<-release
	}}
	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("the installer did not start")
	}
	return item, release
}

func TestWorkRunningIsFalseWhenNothingRuns(t *testing.T) {
	s, _, _ := newTestService(t)
	if s.WorkRunning() {
		t.Fatal("WorkRunning() = true for an idle service")
	}
}

func TestWorkRunningWhileAnInstallerRuns(t *testing.T) {
	s, downloads, _ := newTestService(t)
	item, release := startBlockedInstall(t, s, downloads)
	if !s.WorkRunning() {
		close(release)
		t.Fatal("WorkRunning() = false while an installer runs")
	}
	close(release)
	s.waitJobDone(t, item.ID)
	waitFor(t, "no work after the install", func() bool { return !s.WorkRunning() })
}

func TestWorkRunningWhileAGameIsRemoved(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Game")
	registrar.put(library.Game{ID: "g1", Title: "Game", InstallDir: dir, Executable: filepath.Join(dir, "Game.exe")})
	inside := make(chan bool, 1)
	registrar.setOnRemove(func() { inside <- s.WorkRunning() })

	if err := s.RemoveGame("g1", RemoveOptions{}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !<-inside {
		t.Fatal("WorkRunning() = false while a removal runs")
	}
	if s.WorkRunning() {
		t.Fatal("WorkRunning() = true after the removal finished")
	}
}

func TestWorkRunningWhileABrokerLives(t *testing.T) {
	s, _, _ := newTestService(t)
	_, procs := fakeElevation(t)
	s.HandleDownloadStarted(startedDownload(t, s))
	var proc *fakeProc
	select {
	case proc = <-procs:
	case <-time.After(5 * time.Second):
		t.Fatal("the broker did not start")
	}
	if !s.WorkRunning() {
		t.Fatal("WorkRunning() = false while a pre-elevated broker lives")
	}
	proc.close()
	waitFor(t, "the broker's exit to be noticed", func() bool { return !s.WorkRunning() })
}
