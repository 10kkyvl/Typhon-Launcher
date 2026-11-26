package install

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
)

func (f *fakeRegistrar) setOnRemove(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onRemove = hook
}

func (f *fakeRegistrar) setOnGetGames(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onGetGames = hook
}

// seedRunningJob puts item in the service with a job still registered for it,
// as the job goroutine holds it until endJob; cancelled is what Cancel set.
func seedRunningJob(s *Service, item *Installation, cancelled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
	s.jobs[item.ID] = &job{cancel: func() {}, cancelled: cancelled}
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

// A cancelled install deletes the folder it made from inside its own job,
// after its record already says cancelled and before the job ends: only the
// job says that work runs, and quitting the launcher then would cut the
// deletion short.
func TestWorkRunningWhileACancelledInstallDeletesItsFolder(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := t.TempDir()
	archive := filepath.Join(dir, "game.zip")
	writeZip(t, archive, []zipEntry{{name: "a.txt", data: []byte("a")}})
	dest := filepath.Join(dir, "Games", "Game")
	mkFile(t, filepath.Join(dest, "Game.exe"), 32)
	seedRunningJob(s, &Installation{
		ID: "i1", Type: TypeArchiveZip, Status: StatusExtracting, ArchivePath: archive,
		Destination: dest, OwnedDestination: dest,
	}, true)
	type seen struct {
		status  Status
		running bool
	}
	var mu sync.Mutex
	var during []seen
	registrar.setOnGetGames(func() {
		got, ok := s.snapshot("i1")
		if !ok {
			t.Error("the record of the cancelled install is gone")
		}
		at := seen{status: got.Status, running: s.WorkRunning()}
		mu.Lock()
		during = append(during, at)
		mu.Unlock()
	})

	s.fail("i1", context.Canceled)
	s.wg.Wait()
	registrar.setOnGetGames(nil)
	mu.Lock()
	defer mu.Unlock()

	if len(during) == 0 {
		t.Fatal("setup: the cancelled install never checked the library before deleting its folder")
	}
	for _, at := range during {
		if at.status != StatusCancelled {
			t.Fatalf("setup: the folder was checked with the record %s, want it already cancelled", at.status)
		}
		if !at.running {
			t.Fatal("WorkRunning() = false while a cancelled install deletes its folder in its job")
		}
	}
	if exists(dest) {
		t.Fatalf("setup: the destination %s survived the cancel", dest)
	}
}

// A retried install is pending, with no job yet, while its old .partial
// folder is swept: a record in a working status is work by itself.
func TestWorkRunningWhileARetriedInstallSweepsItsPartialFolder(t *testing.T) {
	r := newRig(t)
	archive := zipDownload(t, r)
	dest := filepath.Join(t.TempDir(), "Game")
	seedFailedInstall(t, r, archive, dest, true)
	mkFile(t, filepath.Join(dest+partialSuffix, "data.bin"), 64)
	started, release := blockPartialRemoval(t)

	retried := make(chan error, 1)
	go func() { retried <- r.s.Retry("failed1") }()
	select {
	case <-started:
	case <-time.After(15 * time.Second):
		t.Fatal("the retry never swept the partial folder")
	}
	running := r.s.WorkRunning()
	release()
	if err := <-retried; err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.settle("failed1")
	if !running {
		t.Fatal("WorkRunning() = false while a retried install is pending")
	}
}

// raiseBroker starts the pre-elevated broker of download d1 and waits for it.
func raiseBroker(t *testing.T, s *Service) *fakeProc {
	t.Helper()
	_, procs := fakeElevation(t)
	s.HandleDownloadStarted(startedDownload(t, s))
	select {
	case proc := <-procs:
		return proc
	case <-time.After(5 * time.Second):
		t.Fatal("the broker did not start")
		return nil
	}
}

// A broker raised ahead of an install lives for the whole download (up to
// brokerMaxLifetime) and stops within seconds at shutdown: while it waits
// for a job it is not work that keeps the launcher from updating.
func TestAnIdleBrokerIsNotWork(t *testing.T) {
	s, _, _ := newTestService(t)
	raiseBroker(t, s)
	if s.brokerFor("d1") == nil {
		t.Fatal("setup: the broker is not registered")
	}
	if s.WorkRunning() {
		t.Fatal("WorkRunning() = true for a broker that runs no job")
	}
}

// A broker that runs an installer is the install's job: the shutdown waits
// for that installer.
func TestWorkRunningWhileABrokerRunsAnInstaller(t *testing.T) {
	s, downloads, _ := newTestService(t)
	raiseBroker(t, s)
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "setup.exe"), 4096)
	downloads.add("d1", "Game", root)
	s.roots = []string{t.TempDir()}
	release := make(chan struct{})
	entered := make(chan bool, 1)
	s.runner = &fakeRunner{act: func(spec runSpec) {
		entered <- spec.Broker != nil
		<-release
	}}
	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case viaBroker := <-entered:
		if !viaBroker {
			close(release)
			t.Fatal("setup: the installer did not go to the broker")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the installer did not start")
	}
	if !s.WorkRunning() {
		close(release)
		t.Fatal("WorkRunning() = false while the broker runs an installer")
	}
	close(release)
	s.waitJobDone(t, item.ID)
}
