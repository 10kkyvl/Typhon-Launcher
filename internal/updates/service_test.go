package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/sources"
)

type fakeLibrary struct {
	mu      sync.Mutex
	games   []library.Game
	running []string
	applied []library.InstalledUpdate
}

func (f *fakeLibrary) GetInstalledGames() []library.Game {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]library.Game(nil), f.games...)
}

func (f *fakeLibrary) GetRunningGames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.running...)
}

func (f *fakeLibrary) ApplyInstalledUpdate(u library.InstalledUpdate) (library.Game, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, u)
	for i := range f.games {
		if f.games[i].ID != u.ID {
			continue
		}
		f.games[i].Version = u.Version
		f.games[i].Executable = u.Executable
		f.games[i].ReleaseID = u.ReleaseID
		return f.games[i], nil
	}
	return library.Game{}, errors.New("not found")
}

type fakeReleases struct{ list []sources.Release }

func (f *fakeReleases) ReleasesFor(string, string) []sources.Release {
	return append([]sources.Release(nil), f.list...)
}

func (f *fakeReleases) FindRelease(id string) (sources.Release, bool) {
	for _, r := range f.list {
		if r.ID == id {
			return r, true
		}
	}
	return sources.Release{}, false
}

type fakeDownloads struct {
	mu       sync.Mutex
	tasks    map[string]*download.Download
	addErr   error
	failTask bool
	requests []download.AddRequest
}

func newFakeDownloads() *fakeDownloads {
	return &fakeDownloads{tasks: map[string]*download.Download{}}
}

func (f *fakeDownloads) AddTask(_ context.Context, req download.AddRequest) (download.Download, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return download.Download{}, f.addErr
	}
	f.requests = append(f.requests, req)
	task := &download.Download{
		ID:          "task-" + req.Origin.ReleaseID,
		Name:        req.Name,
		Destination: req.Destination,
		Status:      download.StatusCompleted,
		Progress:    1,
		Origin:      req.Origin,
		Flat:        req.Flat,
		InPlace:     req.InPlace,
	}
	if f.failTask {
		task.Status = download.StatusFailed
		task.Error = "сеть недоступна"
	}
	f.tasks[task.ID] = task
	return *task, nil
}

func (f *fakeDownloads) InspectReuse(context.Context, download.ReuseRequest, func(download.VerifyProgress)) (download.ReuseReport, error) {
	return download.ReuseReport{}, errors.New("недоступно")
}

func (f *fakeDownloads) Get(id string) (download.Download, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[id]
	if !ok {
		return download.Download{}, errors.New("not found")
	}
	return *task, nil
}

func (f *fakeDownloads) Cancel(string) error { return nil }

func (f *fakeDownloads) ByOrigin(gameID string, purpose download.Purpose) []download.Download {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []download.Download
	for _, task := range f.tasks {
		if task.Origin.Purpose == purpose && task.Origin.LibraryID == gameID {
			out = append(out, *task)
		}
	}
	return out
}

type fakeInstaller struct {
	service *Service
	files   map[string]string
	fail    bool
}

func (f *fakeInstaller) Start(downloadID string, opts install.StartOptions) (install.Installation, error) {
	item := install.Installation{
		ID:          "install-" + downloadID,
		DownloadID:  downloadID,
		Destination: opts.Destination,
		Status:      install.StatusCompleted,
	}
	if f.fail {
		item.Status = install.StatusFailed
		item.Error = "установка не удалась"
	} else {
		for rel, content := range f.files {
			path := filepath.Join(opts.Destination, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return install.Installation{}, err
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return install.Installation{}, err
			}
		}
		item.Executable = filepath.Join(opts.Destination, "game.exe")
	}
	go f.service.HandleInstallFinished(item)
	return item, nil
}

type harness struct {
	service    *Service
	library    *fakeLibrary
	releases   *fakeReleases
	downloads  *fakeDownloads
	installer  *fakeInstaller
	installDir string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	previous := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previous })

	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	writeFile(t, installDir, "game.exe", "old executable")
	writeFile(t, installDir, "data/pak0.pak", "old data")
	writeFile(t, installDir, "saves/profile.sav", "player progress")

	game := library.Game{
		ID:              "local-1",
		Title:           "Game",
		CanonicalGameID: canonical,
		InstallDir:      installDir,
		Executable:      filepath.Join(installDir, "game.exe"),
		ReleaseID:       "r1",
		Version:         "1.0",
		VersionSource:   string(VersionSourceRelease),
	}

	h := &harness{
		library:    &fakeLibrary{games: []library.Game{game}},
		releases:   &fakeReleases{list: []sources.Release{release("r1", "1.0", 10<<20), release("r2", "1.1", 12<<20)}},
		downloads:  newFakeDownloads(),
		installDir: installDir,
	}
	h.service = newServiceAt(filepath.Join(root, "config"), nil)
	h.service.library = h.library
	h.service.releases = h.releases
	h.service.downloads = h.downloads
	h.installer = &fakeInstaller{
		service: h.service,
		files:   map[string]string{"game.exe": "new executable", "data/pak0.pak": "new data"},
	}
	h.service.installs = h.installer
	h.service.ctx, h.service.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		h.service.cancel()
		h.service.wg.Wait()
	})
	return h
}

func (h *harness) plan(t *testing.T) UpdatePlan {
	t.Helper()
	h.service.check(h.library.games[0])
	plan, err := h.service.buildPlan(context.Background(), "local-1")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	h.service.mutate("local-1", func(u *Update) { u.Plan = plan })
	return *plan
}

func (h *harness) waitState(t *testing.T, want State) Update {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		u, ok := h.service.snapshot("local-1")
		if ok && u.State == want {
			return u
		}
		time.Sleep(5 * time.Millisecond)
	}
	u, _ := h.service.snapshot("local-1")
	t.Fatalf("state = %q (%s), want %q", u.State, u.Error, want)
	return Update{}
}

func TestCheckReportsAvailableUpdate(t *testing.T) {
	h := newHarness(t)
	h.service.check(h.library.games[0])
	u, ok := h.service.snapshot("local-1")
	if !ok || u.State != StateAvailable {
		t.Fatalf("update = %+v", u)
	}
	if u.Availability.Kind != KindUpdate || u.Availability.TargetVersion != "1.1" {
		t.Fatalf("availability = %+v", u.Availability)
	}
}

func TestStartUpdateRefusesRunningGame(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	h.library.running = []string{"local-1"}
	if err := h.service.StartUpdate("local-1"); !errors.Is(err, errGameRunning) {
		t.Fatalf("err = %v, want %v", err, errGameRunning)
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation touched: %q %v", data, err)
	}
}

func TestFullReleaseUpdateSwapsAndKeepsUserFiles(t *testing.T) {
	h := newHarness(t)
	plan := h.plan(t)
	if plan.Strategy != StrategyFullRelease {
		t.Fatalf("strategy = %q", plan.Strategy)
	}
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateIdle)

	data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe"))
	if err != nil || string(data) != "new executable" {
		t.Fatalf("executable = %q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "saves", "profile.sav")); err != nil || string(data) != "player progress" {
		t.Fatalf("user file not carried over: %q %v", data, err)
	}
	if _, err := os.Stat(h.installDir + previousSuffix); err != nil {
		t.Fatalf("previous version not kept: %v", err)
	}
	if len(h.library.applied) != 1 || h.library.applied[0].Version != "1.1" {
		t.Fatalf("library update = %+v", h.library.applied)
	}
	if len(h.downloads.requests) != 1 || h.downloads.requests[0].Origin.Purpose != download.PurposeUpdate {
		t.Fatalf("download requests = %+v", h.downloads.requests)
	}
}

func TestUpdateFailureBeforeSwapKeepsInstallation(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	h.downloads.failTask = true
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	u := h.waitState(t, StateFailed)
	if u.Error == "" {
		t.Fatal("expected a failure message")
	}
	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation damaged: %q %v", data, err)
	}
	if _, err := os.Stat(h.installDir + previousSuffix); err == nil {
		t.Fatal("no previous version should exist after a failed download")
	}
}

func TestRollbackRestoresPreviousVersion(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateIdle)

	if err := h.service.Rollback("local-1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe"))
	if err != nil || string(data) != "old executable" {
		t.Fatalf("executable = %q %v", data, err)
	}
	if _, err := os.Stat(h.installDir + previousSuffix); err == nil {
		t.Fatal("previous version should be consumed by the rollback")
	}
	last := h.library.applied[len(h.library.applied)-1]
	if last.Version != "1.0" || last.ReleaseID != "r1" {
		t.Fatalf("library rollback = %+v", last)
	}
	if err := h.service.Rollback("local-1"); !errors.Is(err, errNoRollback) {
		t.Fatalf("second rollback = %v", err)
	}
}

func TestPreviousVersionDroppedAfterSuccessfulLaunch(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateIdle)

	h.service.HandleSessionEnded("local-1", 5)
	if _, err := os.Stat(h.installDir + previousSuffix); err != nil {
		t.Fatal("a short session must not drop the previous version")
	}
	h.service.HandleSessionEnded("local-1", 600)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(h.installDir + previousSuffix); err != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("previous version was not removed after a successful launch")
}

func TestVerifyUnavailableWithoutIdentity(t *testing.T) {
	h := newHarness(t)
	h.releases.list = nil
	if err := h.service.VerifyGame("local-1"); !errors.Is(err, errNoIdentity) {
		t.Fatalf("err = %v, want %v", err, errNoIdentity)
	}
	state, _ := h.service.GetVerifyState("local-1")
	if state.Method != MethodUnavailable {
		t.Fatalf("method = %q", state.Method)
	}
	if err := h.service.RepairGame("local-1"); err == nil {
		t.Fatal("repair must be refused without a torrent identity")
	}
}

func TestSwapAndRestoreDirectories(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "game")
	staging := filepath.Join(root, "staging")
	previous := current + previousSuffix
	writeFile(t, current, "marker", "old")
	writeFile(t, staging, "marker", "new")

	if err := swapDirectories(current, staging, previous); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(current, "marker")); string(data) != "new" {
		t.Fatalf("marker = %q", data)
	}
	if err := restoreDirectories(current, previous); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(current, "marker")); string(data) != "old" {
		t.Fatalf("restored marker = %q", data)
	}
	if _, err := os.Stat(current + replacedSuffix); err == nil {
		t.Fatal("the failed installation should be cleaned up")
	}
}

func TestPatchesFromReleasesFeedIntoPlan(t *testing.T) {
	h := newHarness(t)
	gameID := canonical
	h.releases.list = append(h.releases.list, sources.Release{
		ID:              "p1",
		Kind:            sources.KindPatch,
		CanonicalGameID: &gameID,
		FromVersion:     "1.0",
		ToVersion:       "1.1",
		Size:            1 << 20,
		URIs:            []string{"magnet:?xt=urn:btih:p1"},
		MatchStatus:     catalog.StatusMatched,
		MatchConfidence: 1,
		Availability:    sources.AvailabilityAvailable,
	})
	h.service.check(h.library.games[0])
	plan, err := h.service.buildPlan(context.Background(), "local-1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != StrategyPatchChain || len(plan.Patches) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.DownloadBytes != 1<<20 {
		t.Fatalf("download bytes = %d", plan.DownloadBytes)
	}
}
