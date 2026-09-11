package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/sources"
)

func newPatchScenario(t *testing.T, failReleaseID string) (*Service, *fakeLibrary, string) {
	t.Helper()
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "game.exe"), []byte("v1.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := &fakeLibrary{games: []library.Game{{
		ID: "g1", Title: "Game", InstallDir: installDir,
		Executable: filepath.Join(installDir, "game.exe"), Version: "1.0",
		ReleaseID: "r1", SourceID: "src", DistributionID: "main",
	}}}
	svc, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = lib
	svc.releases = &fakeReleases{list: []sources.Release{patchRelease("p1", "1.0", "1.1", 1<<20), patchRelease("p2", "1.1", "1.2", 1<<20)}}
	svc.downloads = &patchDownloads{fakeDownloads: *newFakeDownloads(), failReleaseID: failReleaseID}
	svc.installs = &patchInstaller{service: svc, content: map[string]map[string]string{
		"task-p1": {"game.exe": "v1.1"},
		"task-p2": {"game.exe": "v1.2"},
	}}
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		svc.cancel()
		svc.wg.Wait()
	})
	previousPoll := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previousPoll })
	return svc, lib, installDir
}

func patchChainPlan() UpdatePlan {
	return UpdatePlan{
		GameID: "g1", Strategy: StrategyPatchChain, InstalledReleaseID: "r1",
		TargetReleaseID: "r2", SourceID: "src", DistributionID: "main",
		InstalledVersion: "1.0", TargetVersion: "1.2",
		Patches: []Patch{
			{ID: "p1", ReleaseID: "p1", SourceID: "src", DistributionID: "main", FromVersion: "1.0", ToVersion: "1.1"},
			{ID: "p2", ReleaseID: "p2", SourceID: "src", DistributionID: "main", FromVersion: "1.1", ToVersion: "1.2"},
		},
	}
}

func rollbackEntry(t *testing.T, svc *Service, gameID string) *Rollback {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	entry, ok := svc.rollbacks[gameID]
	if !ok {
		return nil
	}
	copied := *entry
	return &copied
}

// TestApplyPatchChainOffersRollbackToTheVersionBeforeTheChain covers
// invariant 15 for the patch path: every patch merges into the live
// installation, so the chain has to keep the pre-chain copy and offer it as a
// rollback, the same way a full release does.
func TestApplyPatchChainOffersRollbackToTheVersionBeforeTheChain(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "")

	if err := svc.applyPatchChain(context.Background(), patchChainPlan()); err != nil {
		t.Fatalf("applyPatchChain: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.2" {
		t.Fatalf("game.exe = %q, err = %v, want v1.2", data, err)
	}
	entry := rollbackEntry(t, svc, "g1")
	if entry == nil {
		t.Fatal("no rollback entry after a patch chain: the player has no way back")
	}
	if entry.Version != "1.0" {
		t.Fatalf("rollback version = %q, want the version the chain started from", entry.Version)
	}
	if entry.Path != installDir+previousSuffix {
		t.Fatalf("rollback path = %q, want %q", entry.Path, installDir+previousSuffix)
	}
	if data, err := os.ReadFile(filepath.Join(entry.Path, "game.exe")); err != nil || string(data) != "v1.0" {
		t.Fatalf("backup game.exe = %q, err = %v, want v1.0", data, err)
	}
}

// TestApplyPatchChainKeepsRollbackWhenTheChainBreaksHalfway is the case the
// issue describes: a chain that dies on its second patch leaves the game at
// an intermediate version, and only the pre-chain copy leads back out.
func TestApplyPatchChainKeepsRollbackWhenTheChainBreaksHalfway(t *testing.T) {
	svc, lib, installDir := newPatchScenario(t, "p2")

	chainErr := svc.applyPatchChain(context.Background(), patchChainPlan())
	if chainErr == nil {
		t.Fatal("expected the second patch to fail")
	}

	lib.mu.Lock()
	version := lib.games[0].Version
	lib.mu.Unlock()
	if version != "1.1" {
		t.Fatalf("library version = %q, want the intermediate 1.1", version)
	}
	entry := rollbackEntry(t, svc, "g1")
	if entry == nil {
		t.Fatal("no rollback entry after an interrupted chain")
	}
	if !strings.Contains(chainErr.Error(), "1.1 → 1.2") {
		t.Fatalf("error = %q, want it to name the patch the chain stopped on", chainErr)
	}
	if data, err := os.ReadFile(filepath.Join(entry.Path, "game.exe")); err != nil || string(data) != "v1.0" {
		t.Fatalf("backup game.exe = %q, err = %v, want v1.0", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.1" {
		t.Fatalf("game.exe = %q, err = %v, want v1.1", data, err)
	}
}

// TestApplyPatchChainDropsBackupWhenNothingWasApplied keeps the copy honest
// in the other direction: a chain that failed before touching the
// installation must not leave a full copy behind, nor offer a rollback to the
// version the player is already running.
func TestApplyPatchChainDropsBackupWhenNothingWasApplied(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "p1")

	if err := svc.applyPatchChain(context.Background(), patchChainPlan()); err == nil {
		t.Fatal("expected the first patch to fail")
	}

	if entry := rollbackEntry(t, svc, "g1"); entry != nil {
		t.Fatalf("rollback entry = %+v, want none: nothing was applied", entry)
	}
	if _, err := os.Stat(installDir + previousSuffix); err == nil {
		t.Fatal("pre-chain copy kept after a chain that changed nothing")
	}
}

// TestApplyPatchChainDropsBackupWhenPolicyIsOff mirrors the torrent-reuse
// case: the copy has to exist for the whole chain, and only then does
// KeepPreviousVersion decide its fate.
func TestApplyPatchChainDropsBackupWhenPolicyIsOff(t *testing.T) {
	svc, _, installDir := newPatchScenario(t, "")
	svc.settings = settingsWith(t, func(s *settings.Settings) {
		s.KeepPreviousVersion = settings.KeepPreviousOff
	})

	if err := svc.applyPatchChain(context.Background(), patchChainPlan()); err != nil {
		t.Fatalf("applyPatchChain: %v", err)
	}

	if _, err := os.Stat(installDir + previousSuffix); err == nil {
		t.Fatal("pre-chain copy must be removed when the keep-previous policy is off")
	}
	if entry := rollbackEntry(t, svc, "g1"); entry != nil {
		t.Fatalf("rollback entry = %+v, want none when the policy is off", entry)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.2" {
		t.Fatalf("game.exe = %q, err = %v, want v1.2", data, err)
	}
}

func settingsWith(t *testing.T, apply func(*settings.Settings)) *settings.Service {
	t.Helper()
	svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	next := settings.Defaults()
	apply(&next)
	if err := svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	return svc
}

// TestPlanPromisesSaveBackupOnlyWhenTheLocationIsKnown covers the switch the
// issue found dead: what the plan shows the player has to follow from the
// setting and from a saves folder that was actually located.
func TestPlanPromisesSaveBackupOnlyWhenTheLocationIsKnown(t *testing.T) {
	h := newHarness(t)
	savesDir := filepath.Join(h.installDir, "saves")

	plan := h.plan(t)
	if plan.BackupAvailable || plan.SavesPath != "" {
		t.Fatalf("plan promises a save backup with no known saves folder: %+v", plan)
	}

	h.library.mu.Lock()
	h.library.saves = savesDir
	h.library.mu.Unlock()

	plan = h.plan(t)
	if !plan.BackupAvailable || plan.SavesPath != savesDir {
		t.Fatalf("savesPath = %q, backupAvailable = %v, want %q and true", plan.SavesPath, plan.BackupAvailable, savesDir)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Kind != StepBackup {
		t.Fatalf("first step = %+v, want the save snapshot", plan.Steps)
	}
}

func TestPlanSkipsSaveBackupWhenTheSwitchIsOff(t *testing.T) {
	h := newHarness(t)
	h.library.mu.Lock()
	h.library.saves = filepath.Join(h.installDir, "saves")
	h.library.mu.Unlock()
	h.service.settings = settingsWith(t, func(s *settings.Settings) { s.UpdateSaveBackup = false })

	plan := h.plan(t)
	if plan.BackupAvailable || plan.SavesPath != "" {
		t.Fatalf("plan = %+v, want no save backup when the switch is off", plan)
	}
}

// TestPlanFailsWhenTheSavesLocationCannotBeRead keeps a failed lookup from
// turning into a quiet "no saves here": the player asked for the snapshot.
func TestPlanFailsWhenTheSavesLocationCannotBeRead(t *testing.T) {
	h := newHarness(t)
	h.library.mu.Lock()
	h.library.savesErr = errors.New("папка сохранений недоступна")
	h.library.mu.Unlock()

	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := h.service.buildPlan(context.Background(), "local-1"); err == nil {
		t.Fatal("expected the plan to fail when the saves lookup does")
	}
}

// TestUpdateSnapshotsSavesBeforeTheFirstWrite is the switch actually doing
// what it says: the snapshot exists after the update, outside the
// installation the update rewrote.
func TestUpdateSnapshotsSavesBeforeTheFirstWrite(t *testing.T) {
	h := newHarness(t)
	h.library.mu.Lock()
	h.library.saves = filepath.Join(h.installDir, "saves")
	h.library.mu.Unlock()
	h.plan(t)

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatalf("start update: %v", err)
	}
	h.service.wg.Wait()

	u, ok := h.service.snapshot("local-1")
	if !ok || u.SavesBackup == "" {
		t.Fatalf("update = %+v, want the snapshot path recorded", u)
	}
	data, err := os.ReadFile(filepath.Join(u.SavesBackup, "profile.sav"))
	if err != nil || string(data) != "player progress" {
		t.Fatalf("snapshot profile.sav = %q, err = %v, want the saved progress", data, err)
	}
	if within, err := filepath.Rel(h.installDir, u.SavesBackup); err == nil && !filepath.IsAbs(within) && within[0] != '.' {
		t.Fatalf("snapshot %q sits inside the installation the update replaces", u.SavesBackup)
	}
}

// TestUpdateStopsWhenTheSaveSnapshotFails covers the same promise from the
// other side: a snapshot that cannot be taken stops the update while the
// installation is still untouched, instead of updating without it.
func TestUpdateStopsWhenTheSaveSnapshotFails(t *testing.T) {
	h := newHarness(t)
	h.library.mu.Lock()
	h.library.saves = filepath.Join(t.TempDir(), "gone")
	h.library.mu.Unlock()
	h.plan(t)

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatalf("start update: %v", err)
	}
	h.service.wg.Wait()

	u, ok := h.service.snapshot("local-1")
	if !ok || u.State != StateFailed {
		t.Fatalf("state = %+v, want the update to fail", u)
	}
	if len(h.downloads.requests) != 0 {
		t.Fatalf("download requests = %+v, want none: the update must stop before downloading", h.downloads.requests)
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation touched after a failed save snapshot: %q %v", data, err)
	}
}
