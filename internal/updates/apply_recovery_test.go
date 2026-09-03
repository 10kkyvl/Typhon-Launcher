package updates

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/sources"
)

func mkTree(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil {
		t.Fatalf("read marker %s: %v", dir, err)
	}
	return string(data)
}

func TestSwapDirectoriesJournalsBeforeRename(t *testing.T) {
	root := t.TempDir()
	svc, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "game")
	staging := filepath.Join(root, "does-not-exist")
	previous := current + previousSuffix
	mkTree(t, current, "old")

	if err := svc.swapDirectories("g1", current, staging, previous, "2.0"); err == nil {
		t.Fatal("expected error: staging missing")
	}
	if got := readMarker(t, current); got != "old" {
		t.Fatalf("current restored to %q, want old", got)
	}
	journals, err := svc.store.loadJournals()
	if err != nil {
		t.Fatalf("load journals: %v", err)
	}
	if len(journals) != 1 || journals[0].GameID != "g1" || journals[0].Kind != JournalSwap {
		t.Fatalf("journal not written before the swap attempt: %+v", journals)
	}
}

func TestSetJournalRollsBackOnPersistFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil { //nolint:gosec // G302: временно закрываем права каталога, чтобы смоделировать сбой persist (инвариант 5)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(configDir, 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore config dir permissions: %v", err)
		}
	})

	if err := svc.setJournal(SwapJournal{GameID: "g1", Kind: JournalSwap}); err == nil {
		t.Fatal("expected persist error on read-only config dir")
	}
	if _, ok := svc.journals["g1"]; ok {
		t.Fatal("journal must not remain in memory after a failed persist")
	}
}

func TestClearJournalRollsBackOnPersistFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.setJournal(SwapJournal{GameID: "g1", Kind: JournalSwap}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configDir, 0o500); err != nil { //nolint:gosec // G302: временно закрываем права каталога, чтобы смоделировать сбой persist (инвариант 5)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(configDir, 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore config dir permissions: %v", err)
		}
	})

	if err := svc.clearJournal("g1"); err == nil {
		t.Fatal("expected persist error on read-only config dir")
	}
	if _, ok := svc.journals["g1"]; !ok {
		t.Fatal("journal must remain in memory after a failed clear")
	}
}

// crashScenario builds a service wired to a fake library with a single
// installed game, seeds the on-disk shape a crash leaves at a given window
// of the full-release swap, then runs ServiceStartup.
type crashScenario struct {
	root       string
	configDir  string
	installDir string
	previous   string
	staging    string
	svc        *Service
	lib        *fakeLibrary
}

func newCrashScenario(t *testing.T) *crashScenario {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	installDir := filepath.Join(root, "Games", "Game")
	staging, err := stagingDir(installDir, "g1")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	lib := &fakeLibrary{games: []library.Game{{
		ID:         "g1",
		InstallDir: installDir,
		Executable: filepath.Join(installDir, "marker.txt"),
		Version:    "1.0",
	}}}
	svc.library = lib
	return &crashScenario{
		root:       root,
		configDir:  configDir,
		installDir: installDir,
		previous:   installDir + previousSuffix,
		staging:    staging,
		svc:        svc,
		lib:        lib,
	}
}

func (c *crashScenario) seedJournal(t *testing.T) {
	t.Helper()
	if err := c.svc.store.saveJournals([]SwapJournal{{
		GameID:     "g1",
		Kind:       JournalSwap,
		InstallDir: c.installDir,
		Staging:    c.staging,
		Previous:   c.previous,
		Version:    "2.0",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := c.svc.store.saveUpdates([]Update{{GameID: "g1", State: StateUpdating}}); err != nil {
		t.Fatal(err)
	}
}

func (c *crashScenario) startup(t *testing.T) {
	t.Helper()
	if err := c.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := c.svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	})
}

func (c *crashScenario) assertRolledBack(t *testing.T) {
	t.Helper()
	if got := readMarker(t, c.installDir); got != "old" {
		t.Fatalf("installDir marker = %q, want old", got)
	}
	for _, dir := range []string{c.previous, c.staging, c.installDir + replacedSuffix} {
		if _, err := os.Stat(dir); err == nil {
			t.Fatalf("leftover directory %s", dir)
		}
	}
	u, ok := c.svc.snapshot("g1")
	if !ok || u.State != StateFailed || u.Error != interruptedUpdateText {
		t.Fatalf("update state = %+v", u)
	}
	journals, err := c.svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared: %+v", journals)
	}
}

func TestRecoverSwapWindowA_BeforeFirstRename(t *testing.T) {
	c := newCrashScenario(t)
	mkTree(t, c.installDir, "old")
	mkTree(t, c.staging, "new")
	c.seedJournal(t)
	c.startup(t)
	c.assertRolledBack(t)
}

func TestRecoverSwapWindowB_AfterFirstRename(t *testing.T) {
	c := newCrashScenario(t)
	mkTree(t, c.previous, "old")
	mkTree(t, c.staging, "new")
	c.seedJournal(t)
	c.startup(t)
	c.assertRolledBack(t)
}

func TestRecoverSwapWindowC_AfterSecondRename(t *testing.T) {
	c := newCrashScenario(t)
	mkTree(t, c.installDir, "new")
	mkTree(t, c.previous, "old")
	c.seedJournal(t)
	c.startup(t)
	c.assertRolledBack(t)
}

func TestRecoverSwapWindowD_AfterRememberPrevious(t *testing.T) {
	c := newCrashScenario(t)
	mkTree(t, c.installDir, "new")
	mkTree(t, c.previous, "old")
	c.seedJournal(t)
	if err := c.svc.store.saveRollbacks([]Rollback{{
		GameID:     "g1",
		Path:       c.previous,
		InstallDir: c.installDir,
		Version:    "1.0",
	}}); err != nil {
		t.Fatal(err)
	}
	c.startup(t)
	c.assertRolledBack(t)
	if _, ok := c.svc.rollbacks["g1"]; ok {
		t.Fatal("rollback entry pointing at the consumed .previous must be dropped")
	}
}

func TestRecoverSwapMissingRenamePendingRegistration(t *testing.T) {
	c := newCrashScenario(t)
	mkTree(t, c.installDir, "new")
	mkTree(t, c.staging, "new")
	// simulate: previous already gone (KeepPreviousVersion off consumed it)
	// but the second rename never happened, so installDir must be the
	// *destination* of the recovery rename, not a pre-existing tree.
	if err := os.RemoveAll(c.installDir); err != nil {
		t.Fatal(err)
	}
	c.seedJournal(t)
	c.startup(t)

	if got := readMarker(t, c.installDir); got != "new" {
		t.Fatalf("installDir marker = %q, want new", got)
	}
	if _, err := os.Stat(c.staging); err == nil {
		t.Fatal("staging must be gone after recovery")
	}
	u, ok := c.svc.snapshot("g1")
	if !ok || u.State != StateFailed || u.Error == interruptedUpdateText || u.Error == "" {
		t.Fatalf("update state = %+v, want a distinct pending-registration error", u)
	}
	journals, err := c.svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared: %+v", journals)
	}
}

func TestServiceStartupCorruptJournalFails(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "journal.json"), []byte(`{"version":1,"data":[{"gameId":`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("expected ServiceStartup to fail on corrupt journal file")
	}
}

func TestApplyFullReleaseJournalPersistFailureAbortsBeforeRename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
	h := newHarness(t)
	plan := h.plan(t)
	_ = plan
	if err := os.Chmod(filepath.Dir(h.service.store.path("x")), 0o500); err != nil { //nolint:gosec // G302: временно закрываем права каталога, чтобы смоделировать сбой persist (инвариант 5)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(filepath.Dir(h.service.store.path("x")), 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore config dir permissions: %v", err)
		}
	})

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	u := h.waitState(t, StateFailed)
	if u.Error == "" {
		t.Fatal("expected a failure message")
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation touched before journal could be persisted: %q %v", data, err)
	}
}

type patchDownloads struct {
	fakeDownloads
	failReleaseID string
}

func (f *patchDownloads) AddTask(ctx context.Context, req download.AddRequest) (download.Download, error) {
	task, err := f.fakeDownloads.AddTask(ctx, req)
	if err != nil {
		return task, err
	}
	if req.Origin.ReleaseID == f.failReleaseID {
		task.Status = download.StatusFailed
		task.Error = "патч недоступен"
		f.mu.Lock()
		f.tasks[task.ID] = &task
		f.mu.Unlock()
	}
	return task, nil
}

type patchInstaller struct {
	service *Service
	content map[string]map[string]string
}

func (f *patchInstaller) Start(downloadID string, opts install.StartOptions) (install.Installation, error) {
	item := install.Installation{ID: "install-" + downloadID, DownloadID: downloadID, Destination: opts.Destination, Status: install.StatusCompleted}
	for rel, data := range f.content[downloadID] {
		path := filepath.Join(opts.Destination, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return install.Installation{}, err
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			return install.Installation{}, err
		}
	}
	item.Executable = filepath.Join(opts.Destination, "game.exe")
	go f.service.HandleInstallFinished(item)
	return item, nil
}

// TestApplyPatchChainCommitsEachPatchBeforeTheNext covers invariant 14: a
// chain interrupted on its second patch must leave the library registered at
// the first patch's version, not the original one and not the target.
func TestApplyPatchChainCommitsEachPatchBeforeTheNext(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "game.exe"), []byte("v1.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := &fakeLibrary{games: []library.Game{{
		ID:         "g1",
		Title:      "Game",
		InstallDir: installDir,
		Executable: filepath.Join(installDir, "game.exe"),
		Version:    "1.0",
	}}}
	downloads := &patchDownloads{fakeDownloads: *newFakeDownloads(), failReleaseID: "p2"}
	svc, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = lib
	svc.releases = &fakeReleases{list: []sources.Release{release("p1", "1.1", 1<<20), release("p2", "1.2", 1<<20)}}
	svc.downloads = downloads
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

	plan := UpdatePlan{
		GameID:        "g1",
		Strategy:      StrategyPatchChain,
		TargetVersion: "1.2",
		Patches: []Patch{
			{ID: "p1", ReleaseID: "p1", FromVersion: "1.0", ToVersion: "1.1"},
			{ID: "p2", ReleaseID: "p2", FromVersion: "1.1", ToVersion: "1.2"},
		},
	}

	if err := svc.applyPatchChain(context.Background(), plan); err == nil {
		t.Fatal("expected the second patch to fail")
	}

	lib.mu.Lock()
	applied := append([]library.InstalledUpdate(nil), lib.applied...)
	version := lib.games[0].Version
	lib.mu.Unlock()
	if len(applied) != 1 || applied[0].Version != "1.1" {
		t.Fatalf("applied = %+v, want exactly the first patch's version", applied)
	}
	if version != "1.1" {
		t.Fatalf("library version = %q, want 1.1", version)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.1" {
		t.Fatalf("game.exe = %q, err = %v, want v1.1", data, err)
	}
	journals, err := svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared after the first patch committed: %+v", journals)
	}
}

// TestRecoverPatchJournalRestoresBackupAndMarksFailed builds the on-disk
// shape a crash leaves mid-MergeDirWithBackup and checks that ServiceStartup
// restores the pre-patch tree byte-for-byte via RestoreMergeBackup.
func TestRecoverPatchJournalRestoresBackupAndMarksFailed(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	backup := installDir + patchBackupSuffix
	if err := os.MkdirAll(filepath.Join(installDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "game.exe"), []byte("v1.1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "data", "pak0.pak"), []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}

	patchSrc := filepath.Join(root, "patch-src")
	if err := os.MkdirAll(filepath.Join(patchSrc, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchSrc, "game.exe"), []byte("v1.2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchSrc, "data", "pak1.pak"), []byte("new data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchSrc, "data", "pak2.pak"), []byte("even newer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install.MergeDirWithBackup(context.Background(), patchSrc, installDir, backup, nil); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between copyFile finishing pak2.pak's tmp file and the
	// rename that would have made it final.
	if err := os.Remove(filepath.Join(installDir, "data", "pak2.pak")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "data", "pak2.pak.typhon-tmp"), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}

	configDir := filepath.Join(root, "config")
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{{ID: "g1", InstallDir: installDir, Version: "1.1"}}}

	if err := svc.store.saveJournals([]SwapJournal{{
		GameID:     "g1",
		Kind:       JournalPatch,
		InstallDir: installDir,
		Previous:   backup,
		Version:    "1.2",
		Patch:      "p2",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.saveUpdates([]Update{{GameID: "g1", State: StateUpdating}}); err != nil {
		t.Fatal(err)
	}

	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
	})

	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "v1.1" {
		t.Fatalf("game.exe = %q, err = %v, want v1.1", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(installDir, "data", "pak0.pak")); err != nil || string(data) != "untouched" {
		t.Fatalf("pak0.pak = %q, err = %v, want untouched", data, err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "data", "pak1.pak")); err == nil {
		t.Fatal("pak1.pak added by the interrupted patch should have been removed")
	}
	if _, err := os.Stat(filepath.Join(installDir, "data", "pak2.pak")); err == nil {
		t.Fatal("pak2.pak added by the interrupted patch should have been removed")
	}
	if _, err := os.Stat(filepath.Join(installDir, "data", "pak2.pak.typhon-tmp")); err == nil {
		t.Fatal("leftover tmp file must be removed")
	}
	if exists(backup) {
		t.Fatal("backup directory must be gone")
	}
	u, ok := svc.snapshot("g1")
	if !ok || u.State != StateFailed || !strings.Contains(u.Error, "1.2") {
		t.Fatalf("update state = %+v", u)
	}
	journals, err := svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared: %+v", journals)
	}
}

func newInPlaceScenario(t *testing.T) (*Service, *fakeLibrary, *fakeDownloads, string) {
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
		ID:         "g1",
		Title:      "Game",
		InstallDir: installDir,
		Executable: filepath.Join(installDir, "game.exe"),
		Version:    "1.0",
	}}}
	downloads := newFakeDownloads()
	svc, err := newServiceAt(filepath.Join(root, "config"), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = lib
	svc.releases = &fakeReleases{list: []sources.Release{release("r2", "1.1", 1<<20)}}
	svc.downloads = downloads
	svc.ctx, svc.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		svc.cancel()
		svc.wg.Wait()
	})
	previousPoll := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previousPoll })
	return svc, lib, downloads, installDir
}

func inPlacePlan(gameID string) UpdatePlan {
	return UpdatePlan{GameID: gameID, Strategy: StrategyTorrentReuse, TargetReleaseID: "r2", TargetVersion: "1.1"}
}

// TestApplyTorrentReuseKeepsBackupUntilPolicyDrops covers invariant 15: the
// backup taken before the in-place write survives a successful update and is
// only removed afterward, governed by the same keep-previous policy as a
// full-release swap.
func TestApplyTorrentReuseKeepsBackupUntilPolicyDrops(t *testing.T) {
	svc, _, _, installDir := newInPlaceScenario(t)

	if err := svc.applyTorrentReuse(context.Background(), inPlacePlan("g1")); err != nil {
		t.Fatalf("applyTorrentReuse: %v", err)
	}
	if _, err := os.Stat(installDir + previousSuffix); err != nil {
		t.Fatalf(".previous missing after a successful reuse update: %v", err)
	}
	journals, err := svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared: %+v", journals)
	}
}

func TestApplyTorrentReuseDropsBackupWhenPolicyIsOff(t *testing.T) {
	svc, _, _, installDir := newInPlaceScenario(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsSvc, err := settings.NewServiceAt(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	next := settings.Defaults()
	next.KeepPreviousVersion = settings.KeepPreviousOff
	if err := settingsSvc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	svc.settings = settingsSvc

	if err := svc.applyTorrentReuse(context.Background(), inPlacePlan("g1")); err != nil {
		t.Fatalf("applyTorrentReuse: %v", err)
	}
	if _, err := os.Stat(installDir + previousSuffix); err == nil {
		t.Fatal(".previous must be removed when the keep-previous policy is off")
	}
}

func TestApplyTorrentReuseCopyFailureAbortsBeforeDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
	svc, _, downloads, installDir := newInPlaceScenario(t)
	gamesDir := filepath.Dir(installDir)
	if err := os.Chmod(gamesDir, 0o500); err != nil { //nolint:gosec // G302: временно закрываем права каталога, чтобы смоделировать сбой backup-копии (инвариант 5)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(gamesDir, 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore games dir permissions: %v", err)
		}
	})

	if err := svc.applyTorrentReuse(context.Background(), inPlacePlan("g1")); err == nil {
		t.Fatal("expected the backup copy to fail on a read-only parent directory")
	}
	if len(downloads.requests) != 0 {
		t.Fatalf("download requests = %+v, want none: the update must abort before downloading anything", downloads.requests)
	}
}

// TestRecoverInplaceJournalRestoresBackup builds the on-disk shape a crash
// leaves right after the in-place write started: installDir modified,
// previous holding the pre-write backup.
func TestRecoverInplaceJournalRestoresBackup(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	previous := installDir + previousSuffix
	mkTree(t, installDir, "corrupted-mid-write")
	mkTree(t, previous, "old")

	configDir := filepath.Join(root, "config")
	svc, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{{ID: "g1", InstallDir: installDir, Version: "1.0"}}}
	if err := svc.store.saveJournals([]SwapJournal{{
		GameID: "g1", Kind: JournalInplace, InstallDir: installDir, Previous: previous, Version: "1.1",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.saveUpdates([]Update{{GameID: "g1", State: StateUpdating}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.saveRollbacks([]Rollback{{GameID: "g1", Path: previous, InstallDir: installDir, Version: "1.0"}}); err != nil {
		t.Fatal(err)
	}

	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
	})

	if got := readMarker(t, installDir); got != "old" {
		t.Fatalf("installDir marker = %q, want old", got)
	}
	if exists(previous) {
		t.Fatal(".previous must be consumed by the restore")
	}
	u, ok := svc.snapshot("g1")
	if !ok || u.State != StateFailed || u.Error != interruptedUpdateText {
		t.Fatalf("update state = %+v", u)
	}
	if _, ok := svc.rollbacks["g1"]; ok {
		t.Fatal("stale rollback entry pointing at the consumed .previous must be dropped")
	}
	journals, err := svc.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 0 {
		t.Fatalf("journal not cleared: %+v", journals)
	}
}
