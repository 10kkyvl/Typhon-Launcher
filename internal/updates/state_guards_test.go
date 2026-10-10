package updates

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/sources"
)

func storedUpdate(t *testing.T, f *persistFixture, id string) Update {
	t.Helper()
	list, err := f.svc.store.loadUpdates()
	if err != nil {
		t.Fatalf("load updates.json: %v", err)
	}
	for _, u := range list {
		if u.GameID == id {
			return u
		}
	}
	t.Fatalf("updates.json has no entry for %s: %+v", id, list)
	return Update{}
}

func TestForgetPreviousKeepsTheRecordAndStaysDegradedWhenTheWriteFails(t *testing.T) {
	f := newPersistFixture(t)
	f.track(t)
	f.seedRollback(t, Rollback{AwaitLaunch: true})
	f.refuseWrites(t, "rollbacks.json")

	if f.svc.forgetPrevious(f.game.ID) {
		t.Fatal("forgetPrevious reported the record gone although rollbacks.json refused the write")
	}

	if _, ok := f.rollbackEntry(f.game.ID); !ok {
		t.Fatal("the record was dropped in memory although the removal was not saved")
	}
	if u, _ := f.svc.snapshot(f.game.ID); !u.CanRollback {
		t.Fatal("CanRollback cleared in memory while the record still exists")
	}
	if !storedUpdate(t, f, f.game.ID).CanRollback {
		t.Fatal("updates.json says there is no rollback while rollbacks.json still has the record")
	}
	if !f.degraded() {
		t.Fatal("a later successful write cleared the degraded flag of the write that failed")
	}
}

func TestSettlePreviousKeepsTheCopyWhenTheRecordCannotBeDropped(t *testing.T) {
	f := newPersistFixture(t)
	f.svc.settings = settingsWith(t, func(s *settings.Settings) { s.KeepPreviousVersion = settings.KeepPreviousOff })
	f.track(t)
	dir := f.seedRollback(t, Rollback{AwaitLaunch: true})
	f.refuseWrites(t, "rollbacks.json")

	f.svc.settlePrevious(f.game.ID, dir)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the copy was deleted while its record still offers it as a rollback: %v", err)
	}
	if _, ok := f.rollbackEntry(f.game.ID); !ok {
		t.Fatal("the record was dropped in memory although the removal was not saved")
	}
	if !f.degraded() {
		t.Fatal("the service does not report that its state could not be saved")
	}
}

func TestStateIsNotWrittenAfterAFailedStartup(t *testing.T) {
	const corrupt = `{"version":1,"data":[{"gameId":`
	acts := map[string]func(t *testing.T, svc *Service){
		"shutdown": func(t *testing.T, svc *Service) {
			if err := svc.ServiceShutdown(); err != nil {
				t.Errorf("ServiceShutdown: %v", err)
			}
		},
		"check": func(t *testing.T, svc *Service) {
			if _, err := svc.CheckGame("g1"); err == nil {
				t.Error("CheckGame reported success although nothing could be saved")
			}
		},
		"history": func(_ *testing.T, svc *Service) {
			svc.appendHistory(UpdateHistory{ID: "h1", GameID: "g1", Status: HistoryRunning})
		},
		"update": func(_ *testing.T, svc *Service) {
			svc.mu.Lock()
			svc.updates["g1"] = &Update{GameID: "g1"}
			svc.mu.Unlock()
			svc.mutate("g1", func(u *Update) { u.Message = "changed" })
		},
	}
	for name, act := range acts {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			seed, err := newServiceAt(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := seed.store.saveUpdates([]Update{{GameID: "g1", Title: "G"}}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "verify.json"), []byte(corrupt), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshotUpdatesDir(t, dir)

			svc, err := newServiceAt(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc.library = &fakeLibrary{games: []library.Game{{ID: "g1", Title: "G", Version: "1.0", ReleaseID: "r1", SourceID: "src", DistributionID: "main"}}}
			svc.releases = &fakeReleases{list: []sources.Release{release("r1", "1.0", 10<<20)}}
			if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
				t.Fatal("ServiceStartup succeeded on a corrupt verify.json")
			}

			act(t, svc)

			if after := snapshotUpdatesDir(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatalf("a failed startup was followed by a write\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestStoreWritesAgainOnceTheStateWasLoaded(t *testing.T) {
	dir := t.TempDir()
	svc, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(dir, "verify.json")
	if err := os.WriteFile(corrupt, []byte(`{"version":1,"data":[{"gameId":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("ServiceStartup succeeded on a corrupt verify.json")
	}
	if err := os.Remove(corrupt); err != nil {
		t.Fatal(err)
	}

	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup after the state was repaired: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	if err := svc.store.saveUpdates([]Update{{GameID: "g1"}}); err != nil {
		t.Fatalf("save after a successful startup: %v", err)
	}
}

type cancelAfterErrCalls struct {
	context.Context
	mu   sync.Mutex
	left int
}

func (c *cancelAfterErrCalls) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.left <= 0 {
		return context.Canceled
	}
	c.left--
	return c.Context.Err()
}

func seedBackupSlot(t *testing.T, svc *Service, installDir string) string {
	t.Helper()
	dir := installDir + previousSuffix
	writeFile(t, dir, "game.exe", "old backup")
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.rollbacks["g1"] = &Rollback{GameID: "g1", Path: dir, InstallDir: installDir, Version: "0.9", AwaitLaunch: true}
	if err := svc.persistRollbacksLocked(); err != nil {
		t.Fatalf("seed rollbacks: %v", err)
	}
	return dir
}

func TestBackupCopyThatStopsMidwayLeavesNoRecordNamingTheRemovedCopy(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "")
	seedBackupSlot(t, svc, installDir)
	probe := &cancelAfterErrCalls{Context: context.Background(), left: 1 << 30}
	if _, err := install.DirSize(probe, installDir); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterErrCalls{Context: context.Background(), left: 1<<30 - probe.left}

	if err := svc.applyPatchChain(ctx, patchChainPlan()); err == nil {
		t.Fatal("applyPatchChain finished although its backup copy was cancelled")
	}

	if entry := rollbackEntry(t, svc, "g1"); entry != nil {
		t.Fatalf("rollback record %+v survived the deletion of the copy it names: Rollback would restore whatever is there", entry)
	}
	stored, err := svc.store.loadRollbacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("rollbacks.json still names %+v", stored)
	}
}

func TestBackupCopyKeepsTheOldCopyWhenItsRecordCannotBeDropped(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "")
	dir := seedBackupSlot(t, svc, installDir)
	refused := filepath.Join(svc.store.dir, "rollbacks.json")
	if err := os.RemoveAll(refused); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(refused, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := svc.applyPatchChain(context.Background(), patchChainPlan()); err == nil {
		t.Fatal("applyPatchChain went on although the record of the old copy could not be dropped")
	}

	if data, err := os.ReadFile(filepath.Join(dir, "game.exe")); err != nil || string(data) != "old backup" {
		t.Fatalf("old copy = %q (%v), want it untouched while its record is still on disk", data, err)
	}
	if entry := rollbackEntry(t, svc, "g1"); entry == nil {
		t.Fatal("the record was dropped in memory although the removal was not saved")
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.0" {
		t.Fatalf("installation = %q (%v), want it untouched", data, err)
	}
}

func expiredRollback(t *testing.T) (*persistFixture, string) {
	t.Helper()
	f := newPersistFixture(t)
	f.track(t)
	past := time.Now().Add(-time.Hour)
	return f, f.seedRollback(t, Rollback{KeepUntil: &past})
}

func TestSweepPreviousLeavesAGameWithARunningJobAlone(t *testing.T) {
	f, dir := expiredRollback(t)
	if _, started := f.svc.beginJob(f.game.ID); !started {
		t.Fatal("setup: the job did not start")
	}

	f.svc.sweepPrevious()

	if _, ok := f.rollbackEntry(f.game.ID); !ok {
		t.Fatal("the record was swept while a job of this game runs")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the copy was deleted while a job of this game runs: %v", err)
	}

	f.svc.endJob(f.game.ID)
	f.svc.sweepPrevious()

	if _, ok := f.rollbackEntry(f.game.ID); ok {
		t.Fatal("the expired record stayed after the job ended")
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("the expired copy stayed after the job ended")
	}
}

func TestSweepDoesNotDeleteTheBackupOfAChainInFlight(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "")
	svc.releases = &fakeReleases{list: chainReleases()}
	svc.downloads = &hangingDownloads{fakeDownloads: newFakeDownloads(), hang: "p2"}
	prepareChain(t, svc)
	if err := svc.StartUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.DownloadID == "task-p2" })
	expired := time.Now().Add(-time.Hour)
	svc.mu.Lock()
	entry := svc.rollbacks["g1"]
	if entry == nil {
		svc.mu.Unlock()
		t.Fatal("setup: the chain registered no backup")
	}
	entry.KeepUntil = &expired
	svc.mu.Unlock()

	svc.sweepPrevious()

	if rollbackEntry(t, svc, "g1") == nil {
		t.Fatal("the record of the chain's backup was swept while the chain still runs")
	}
	if data, err := os.ReadFile(filepath.Join(installDir+previousSuffix, "game.exe")); err != nil || string(data) != "v1.0" {
		t.Fatalf("backup = %q (%v), want the pre-chain copy while the chain still runs", data, err)
	}
	if err := svc.CancelUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateFailed && !svc.Busy("g1") })
}

func TestHandleSessionEndedLeavesAGameWithARunningJobAlone(t *testing.T) {
	f := newPersistFixture(t)
	f.track(t)
	dir := f.seedRollback(t, Rollback{AwaitLaunch: true})
	if _, started := f.svc.beginJob(f.game.ID); !started {
		t.Fatal("setup: the job did not start")
	}

	f.svc.HandleSessionEnded(f.game.ID, 600)

	if _, ok := f.rollbackEntry(f.game.ID); !ok {
		t.Fatal("the record was dropped while a job of this game runs")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the copy was deleted while a job of this game runs: %v", err)
	}

	f.svc.endJob(f.game.ID)
	f.svc.HandleSessionEnded(f.game.ID, 600)

	if _, ok := f.rollbackEntry(f.game.ID); ok {
		t.Fatal("the record stayed after the job ended and a session finished")
	}
}
