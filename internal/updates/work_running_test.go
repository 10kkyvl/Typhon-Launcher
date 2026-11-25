package updates

import (
	"testing"
	"time"

	"typhon/internal/install"
)

// blockingInstaller holds every unpack of a patch until release is closed.
type blockingInstaller struct {
	*patchInstaller
	entered chan string
	release chan struct{}
}

func (b *blockingInstaller) Start(downloadID string, opts install.StartOptions) (install.Installation, error) {
	select {
	case b.entered <- downloadID:
	default:
	}
	<-b.release
	return b.patchInstaller.Start(downloadID, opts)
}

// newPatchRun is a two-patch chain whose download of hang never ends and
// whose unpacking waits for the returned installer's release.
func newPatchRun(t *testing.T, hang string) (*Service, *blockingInstaller) {
	t.Helper()
	svc, _, _ := newPatchScenario(t, "")
	svc.releases = &fakeReleases{list: chainReleases()}
	svc.downloads = &hangingDownloads{fakeDownloads: newFakeDownloads(), hang: hang}
	inner, ok := svc.installs.(*patchInstaller)
	if !ok {
		t.Fatalf("setup: installer is %T", svc.installs)
	}
	b := &blockingInstaller{patchInstaller: inner, entered: make(chan string, 1), release: make(chan struct{})}
	svc.installs = b
	prepareChain(t, svc)
	return svc, b
}

// A patch being unpacked and applied writes the game's folder; the launcher's
// shutdown would cancel it and wait for the files to be put back, so it is
// work the self-update must not quit the launcher under.
func TestWorkRunningWhileAPatchIsApplied(t *testing.T) {
	svc, inst := newPatchRun(t, "")
	if svc.WorkRunning() {
		t.Fatal("WorkRunning() = true before the update started")
	}
	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-inst.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the update did not reach its first patch")
	}
	running := svc.WorkRunning()
	close(inst.release)
	// The update ends before any verdict: newPatchScenario's cleanup would
	// otherwise race a job still polling.
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateIdle && !svc.Busy("g1") })
	if !running {
		t.Fatal("WorkRunning() = false while a patch is unpacked")
	}
	if svc.WorkRunning() {
		t.Fatal("WorkRunning() = true after the update finished")
	}
}

// An update waiting on its download is not work: the download resumes after
// a restart, and the update's job stops at once.
func TestAnUpdateWaitingOnItsDownloadIsNotWork(t *testing.T) {
	svc, inst := newPatchRun(t, "p1")
	close(inst.release)
	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.DownloadID == "task-p1" && u.Step == StepDownload })
	busy, running := svc.Busy("g1"), svc.WorkRunning()
	if err := svc.CancelUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(Update) bool { return !svc.Busy("g1") })
	if !busy {
		t.Fatal("setup: the update's job was not running")
	}
	if running {
		t.Fatal("WorkRunning() = true for an update that waits on its download")
	}
}
