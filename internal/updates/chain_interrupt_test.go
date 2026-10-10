package updates

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"typhon/internal/download"
)

func (f *hangingDownloads) finish(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task := f.tasks[id]
	task.Status = download.StatusCompleted
	task.Progress = 1
}

func assertChainStoppedAt11(t *testing.T, svc *Service, got Update, wantInError ...string) {
	t.Helper()
	if got.State != StateFailed {
		t.Fatalf("state = %q, want %q: the chain stopped on its second patch and must not look like a clean offer", got.State, StateFailed)
	}
	for _, want := range wantInError {
		if !strings.Contains(got.Error, want) {
			t.Fatalf("Error = %q, want it to name %q", got.Error, want)
		}
	}
	if got.Plan != nil {
		t.Fatalf("plan from before the chain survived: %+v", got.Plan)
	}
	if !got.Availability.Available || got.Availability.InstalledVersion != "1.1" || got.Availability.TargetVersion != "1.2" {
		t.Fatalf("availability = %+v, want 1.1 -> 1.2 from the committed patch", got.Availability)
	}
	if got.Step != "" || got.Progress != 0 || got.Message != "" || got.DownloadID != "" {
		t.Fatalf("state still carries the finished run: step=%q progress=%v message=%q download=%q", got.Step, got.Progress, got.Message, got.DownloadID)
	}
	if !got.CanRollback {
		t.Fatal("the way back to the version the chain started from was dropped")
	}
	history := svc.GetHistory("g1")
	if len(history) != 1 || history[0].Status != HistoryFailed || history[0].Error != got.Error {
		t.Fatalf("history = %+v, want one failed entry carrying %q", history, got.Error)
	}
}

func finishChainFromTheCommittedVersion(t *testing.T, svc *Service, lib *fakeLibrary, installDir string) {
	t.Helper()
	if err := svc.StartUpdate("g1"); err == nil {
		t.Fatal("StartUpdate started without a plan, so it would run against the stale one")
	}
	if err := svc.PreparePlan("g1"); err != nil {
		t.Fatalf("PreparePlan right after the chain stopped: %v", err)
	}
	planned := waitUpdate(t, svc, "g1", func(u Update) bool { return u.Plan != nil && !u.Planning && !svc.Busy("g1") })
	if planned.Plan.InstalledVersion != "1.1" || len(planned.Plan.Patches) != 1 || planned.Plan.Patches[0].ID != "p2" {
		t.Fatalf("plan = %+v, want only p2 from 1.1", planned.Plan)
	}
	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatalf("StartUpdate from the committed version: %v", err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateIdle && !svc.Busy("g1") })
	if got := appliedVersions(lib); !slices.Equal(got, []string{"1.1", "1.2"}) {
		t.Fatalf("library saw versions %v, want 1.1 then 1.2", got)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.2" {
		t.Fatalf("game.exe = %q (%v), want v1.2", data, err)
	}
}

func TestPatchChainCancelledAfterAPatchReportsTheCommittedVersion(t *testing.T) {
	svc, lib, installDir := newPatchScenario(t, "")
	svc.releases = &fakeReleases{list: chainReleases()}
	hanging := &hangingDownloads{fakeDownloads: newFakeDownloads(), hang: "p2"}
	svc.downloads = hanging
	prepareChain(t, svc)

	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.DownloadID == "task-p2" })
	if got := appliedVersions(lib); !slices.Equal(got, []string{"1.1"}) {
		t.Fatalf("setup: library saw versions %v, want the first patch committed", got)
	}
	if err := svc.CancelUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	got := waitUpdate(t, svc, "g1", func(u Update) bool { return u.Error != "" && !svc.Busy("g1") })

	assertChainStoppedAt11(t, svc, got, "1.1 → 1.2", interruptedUpdateText)

	hanging.finish("task-p2")
	hanging.hang = ""
	finishChainFromTheCommittedVersion(t, svc, lib, installDir)
}

func TestPatchChainFailedAfterAPatchReportsTheCommittedVersion(t *testing.T) {
	svc, lib, installDir := newPatchScenario(t, "p2")
	svc.releases = &fakeReleases{list: chainReleases()}
	prepareChain(t, svc)

	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	got := waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateFailed && !svc.Busy("g1") })

	assertChainStoppedAt11(t, svc, got, "1.1 → 1.2", "патч недоступен")

	dl, ok := svc.downloads.(*patchDownloads)
	if !ok {
		t.Fatal("setup: unexpected downloads type")
	}
	dl.mu.Lock()
	dl.failReleaseID = ""
	delete(dl.tasks, "task-p2")
	dl.mu.Unlock()
	finishChainFromTheCommittedVersion(t, svc, lib, installDir)
}
