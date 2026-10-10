package updates

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/sources"
)

type hangingDownloads struct {
	*fakeDownloads
	hang      string
	cancelled []string
	cancelMu  sync.Mutex
}

func (f *hangingDownloads) AddTask(ctx context.Context, req download.AddRequest) (download.Download, error) {
	task, err := f.fakeDownloads.AddTask(ctx, req)
	if err != nil {
		return task, err
	}
	if req.Origin.ReleaseID == f.hang {
		task.Status = download.StatusDownloading
		task.Progress = 0.25
		f.mu.Lock()
		f.tasks[task.ID] = &task
		f.mu.Unlock()
	}
	return task, nil
}

func (f *hangingDownloads) Cancel(id string) error {
	f.cancelMu.Lock()
	defer f.cancelMu.Unlock()
	f.cancelled = append(f.cancelled, id)
	return nil
}

func (f *hangingDownloads) cancelledIDs() []string {
	f.cancelMu.Lock()
	defer f.cancelMu.Unlock()
	return append([]string(nil), f.cancelled...)
}

func chainReleases() []sources.Release {
	return []sources.Release{
		release("r1", "1.0", 50<<20),
		release("r2", "1.2", 100<<20),
		patchRelease("p1", "1.0", "1.1", 1<<20),
		patchRelease("p2", "1.1", "1.2", 1<<20),
	}
}

func prepareChain(t *testing.T, svc *Service) *UpdatePlan {
	t.Helper()
	games := svc.library.GetInstalledGames()
	if len(games) != 1 {
		t.Fatalf("setup: %d installed games", len(games))
	}
	if err := svc.check(games[0]); err != nil {
		t.Fatalf("check: %v", err)
	}
	plan, err := svc.buildPlan(context.Background(), "g1")
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if plan.Strategy != StrategyPatchChain || len(plan.Patches) == 0 {
		t.Fatalf("plan = %+v, want a patch chain", plan)
	}
	svc.mutate("g1", func(u *Update) { u.Plan = plan })
	return plan
}

func appliedVersions(lib *fakeLibrary) []string {
	lib.mu.Lock()
	defer lib.mu.Unlock()
	out := make([]string, 0, len(lib.applied))
	for _, u := range lib.applied {
		out = append(out, u.Version)
	}
	return out
}

func TestBrokenPatchChainFailsWithItsStepAndResumesFromTheLastCommittedPatch(t *testing.T) {
	svc, lib, installDir := newPatchScenario(t, "p2")
	svc.releases = &fakeReleases{list: chainReleases()}
	dl, ok := svc.downloads.(*patchDownloads)
	if !ok {
		t.Fatal("setup: unexpected downloads type")
	}
	plan := prepareChain(t, svc)
	if got := []string{plan.Patches[0].ID, plan.Patches[1].ID}; !slices.Equal(got, []string{"p1", "p2"}) {
		t.Fatalf("plan patches = %v, want p1 then p2", got)
	}

	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	failed := waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateFailed && !svc.Busy("g1") })

	if !strings.Contains(failed.Error, "1.1 → 1.2") || !strings.Contains(failed.Error, "патч недоступен") {
		t.Fatalf("Error = %q, want the patch the chain stopped on and why", failed.Error)
	}
	if failed.Step != "" || failed.Progress != 0 {
		t.Fatalf("a failed update still shows a step: step=%q progress=%v", failed.Step, failed.Progress)
	}
	if got := appliedVersions(lib); !slices.Equal(got, []string{"1.1"}) {
		t.Fatalf("library saw versions %v, want only the committed first patch", got)
	}
	if !failed.CanRollback {
		t.Fatal("the way back to the version the chain started from was not offered")
	}
	history := svc.GetHistory("g1")
	if len(history) != 1 || history[0].Status != HistoryFailed || history[0].Error != failed.Error || history[0].CompletedAt == nil {
		t.Fatalf("history = %+v, want one failed entry carrying the same reason", history)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.1" {
		t.Fatalf("game.exe = %q (%v), want the first patch applied", data, err)
	}

	dl.mu.Lock()
	dl.failReleaseID = ""
	dl.mu.Unlock()
	retried, err := svc.CheckGame("g1")
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Availability.Available || retried.Availability.InstalledVersion != "1.1" || retried.State != StateAvailable {
		t.Fatalf("after the failure the offer = %+v, want 1.1 -> 1.2 from the committed version", retried.Availability)
	}
	if retried.Plan != nil {
		t.Fatalf("the plan from the failed run survived: %+v", retried.Plan)
	}
	resumed, err := svc.buildPlan(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Patches) != 1 || resumed.Patches[0].ID != "p2" || resumed.InstalledVersion != "1.1" {
		t.Fatalf("resumed plan = %+v, want only p2 from 1.1", resumed)
	}
	svc.mutate("g1", func(u *Update) { u.Plan = resumed })
	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateIdle && !svc.Busy("g1") })

	if got := appliedVersions(lib); !slices.Equal(got, []string{"1.1", "1.2"}) {
		t.Fatalf("library saw versions %v, want 1.1 then 1.2", got)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.2" {
		t.Fatalf("game.exe = %q (%v), want v1.2", data, err)
	}
}

func TestCancelUpdateStopsTheDownloadAndKeepsTheInstallation(t *testing.T) {
	h := newHarness(t)
	hanging := &hangingDownloads{fakeDownloads: h.downloads, hang: "r2"}
	h.service.downloads = hanging
	h.plan(t)

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	running := waitUpdate(t, h.service, "local-1", func(u Update) bool {
		return u.State == StateUpdating && u.Step == StepDownload && u.DownloadID != ""
	})

	if err := h.service.CancelUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	got := waitUpdate(t, h.service, "local-1", func(u Update) bool {
		return u.State == StateAvailable && !h.service.Busy("local-1")
	})

	if got.Error != interruptedUpdateText || got.Step != "" || got.Progress != 0 {
		t.Fatalf("cancelled update = %+v, want the interruption reason and no step", got)
	}
	if got.Plan == nil {
		t.Fatal("cancelling dropped the prepared plan, so the player has to plan again")
	}
	if !slices.Contains(hanging.cancelledIDs(), running.DownloadID) {
		t.Fatalf("download %q was not cancelled: %v", running.DownloadID, hanging.cancelledIDs())
	}
	history := h.service.GetHistory("local-1")
	if len(history) != 1 || history[0].Status != HistoryFailed || history[0].Error != interruptedUpdateText {
		t.Fatalf("history = %+v, want one entry marked interrupted", history)
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation touched: %q %v", data, err)
	}
	if _, err := os.Stat(h.installDir + previousSuffix); err == nil {
		t.Fatal("a cancelled download left a previous-version copy")
	}
	if len(h.library.applied) != 0 {
		t.Fatalf("library was told about a version that never installed: %+v", h.library.applied)
	}
}
