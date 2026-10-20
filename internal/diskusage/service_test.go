package diskusage

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/relocate"
	"typhon/internal/savebackup"
	"typhon/internal/screenshots"
	"typhon/internal/settings"
)

type fakeLibrary struct {
	mu      sync.Mutex
	games   []library.Game
	running map[string]bool
	updates map[string]int64
	updErr  error
}

func (f *fakeLibrary) GetGames() []library.Game {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]library.Game(nil), f.games...)
}

func (f *fakeLibrary) IsRunning(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running[id]
}

func (f *fakeLibrary) UpdateMeasuredSize(id, _ string, size int64, _ bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updErr != nil {
		return false, f.updErr
	}
	if f.updates == nil {
		f.updates = map[string]int64{}
	}
	f.updates[id] = size
	return true, nil
}

type fakeDownloads struct {
	mu       sync.Mutex
	items    []download.Download
	owned    map[string]bool
	partOK   map[string]bool
	pending  []download.Cleanup
	orphanOK map[string]bool
	cleaned  []string
	claimed  []string
	cleanErr error
	deleted  []string
	delErr   error
}

func (f *fakeDownloads) List() []download.Download {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]download.Download(nil), f.items...)
}

func (f *fakeDownloads) Get(id string) (download.Download, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.items {
		if d.ID == id {
			return d, nil
		}
	}
	return download.Download{}, download.ErrNotFound
}

func (f *fakeDownloads) DeleteData(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	for i, d := range f.items {
		if d.ID == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			break
		}
	}
	return f.delErr
}

func (f *fakeDownloads) PartFileVerdict(path string) (bool, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.partOK[path]:
		return true, "", ""
	case f.owned[path]:
		return false, diskreason.OwnedByDownload, "файл принадлежит загрузке"
	}
	return false, diskreason.ForeignPart, "файл не принадлежит лаунчеру"
}

func (f *fakeDownloads) PendingCleanups() []download.Cleanup {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]download.Cleanup(nil), f.pending...)
}

func (f *fakeDownloads) CleanupVerdict(path string) (bool, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orphanOK[path] {
		return true, "", ""
	}
	return false, diskreason.OwnedByDownload, "файлы принадлежат текущей загрузке"
}

func (f *fakeDownloads) ReserveCleanup(path string) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.orphanOK[path] {
		return nil, errors.New("claimed")
	}
	f.claimed = append(f.claimed, path)
	return func() {}, nil
}

func (f *fakeDownloads) RemoveCleanup(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleaned = append(f.cleaned, path)
	return f.cleanErr
}

type fakeInstalls struct {
	mu         sync.Mutex
	items      []install.Installation
	partial    map[string]bool
	busy       map[string]bool
	blocked    map[string]error
	verdicts   int
	reserved   []string
	released   []string
	reserveErr error
	inUse      map[string]bool
}

func (f *fakeInstalls) List() []install.Installation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]install.Installation(nil), f.items...)
}

func (f *fakeInstalls) PartialVerdict(path string) (bool, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verdicts++
	if f.partial[path] {
		return true, "", ""
	}
	return false, diskreason.OwnedByInstall, "идёт установка"
}

func (f *fakeInstalls) ReservePartial(path string) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	if !f.partial[path] {
		return nil, errors.New("job started")
	}
	f.reserved = append(f.reserved, path)
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.released = append(f.released, path)
	}, nil
}

func (f *fakeInstalls) PathInUse(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inUse[path]
}

func (f *fakeInstalls) GameBusy(game library.Game) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy[game.ID]
}

func (f *fakeInstalls) CheckDownloadDeletable(d download.Download) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocked[d.ID]
}

type fakeUpdates struct {
	mu    sync.Mutex
	paths map[string]bool
}

func (f *fakeUpdates) PathInUse(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.paths[path]
}

type fakeRelocations struct {
	mu       sync.Mutex
	list     []relocate.Staging
	err      error
	reserved []string
}

func (f *fakeRelocations) ReserveStaging(path string) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserved = append(f.reserved, path)
	return func() {}, nil
}

func (f *fakeRelocations) Stagings() ([]relocate.Staging, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]relocate.Staging(nil), f.list...), f.err
}

type fakeShots struct {
	mu      sync.Mutex
	usage   []screenshots.Usage
	shots   map[string][]screenshots.Shot
	err     error
	failDel map[string]error
}

func (f *fakeShots) Usage(context.Context) ([]screenshots.Usage, error) { return f.usage, f.err }

func (f *fakeShots) List(gameID string) ([]screenshots.Shot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]screenshots.Shot(nil), f.shots[gameID]...), nil
}

func (f *fakeShots) Delete(gameID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failDel[name]; err != nil {
		return err
	}
	list := f.shots[gameID]
	for i, s := range list {
		if s.Name == name {
			f.shots[gameID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	return nil
}

type fakeSnaps struct {
	mu    sync.Mutex
	usage []savebackup.Usage
	snaps map[string][]savebackup.Snapshot
	err   error
}

func (f *fakeSnaps) Usage(context.Context) ([]savebackup.Usage, error) { return f.usage, f.err }

func (f *fakeSnaps) List(_ context.Context, gameID string) ([]savebackup.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]savebackup.Snapshot(nil), f.snaps[gameID]...), nil
}

func (f *fakeSnaps) Delete(gameID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.snaps[gameID]
	for i, s := range list {
		if s.ID == id {
			f.snaps[gameID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

type rig struct {
	svc       *Service
	lib       *fakeLibrary
	dl        *fakeDownloads
	inst      *fakeInstalls
	rel       *fakeRelocations
	upd       *fakeUpdates
	shots     *fakeShots
	snaps     *fakeSnaps
	games     string
	downloads string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	base := t.TempDir()
	r := &rig{
		lib:       &fakeLibrary{},
		dl:        &fakeDownloads{owned: map[string]bool{}, partOK: map[string]bool{}, orphanOK: map[string]bool{}},
		inst:      &fakeInstalls{partial: map[string]bool{}, busy: map[string]bool{}, blocked: map[string]error{}, inUse: map[string]bool{}},
		rel:       &fakeRelocations{},
		upd:       &fakeUpdates{paths: map[string]bool{}},
		shots:     &fakeShots{shots: map[string][]screenshots.Shot{}, failDel: map[string]error{}},
		snaps:     &fakeSnaps{snaps: map[string][]savebackup.Snapshot{}},
		games:     filepath.Join(base, "games"),
		downloads: filepath.Join(base, "downloads"),
	}
	for _, dir := range []string{r.games, r.downloads} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(Deps{
		Settings: func() settings.Settings {
			s := settings.Defaults()
			s.GamesPath, s.DownloadsPath = r.games, r.downloads
			return s
		},
		Library: r.lib, Downloads: r.dl, Installs: r.inst, Relocations: r.rel, Updates: r.upd, Screenshots: r.shots, Snapshots: r.snaps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	r.svc = svc
	return r
}

// makeLink links a folder the way the platform really does it: Windows
// folders are linked by junctions, which need no privilege.
func makeLink(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		//nolint:gosec // G204: arguments are t.TempDir() paths, no external input (invariant 32 does not apply)
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("junction: %v (%s)", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func writeBytes(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) addGame(t *testing.T, id string, size int) library.Game {
	t.Helper()
	dir := filepath.Join(r.games, id)
	writeBytes(t, filepath.Join(dir, "game.exe"), size)
	g := library.Game{ID: id, Title: "Game " + id, InstallDir: dir, SizeBytes: 1}
	r.lib.games = append(r.lib.games, g)
	return g
}

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	good := Deps{
		Settings: func() settings.Settings { return settings.Defaults() },
		Library:  &fakeLibrary{}, Downloads: &fakeDownloads{}, Installs: &fakeInstalls{},
		Relocations: &fakeRelocations{}, Updates: &fakeUpdates{}, Screenshots: &fakeShots{}, Snapshots: &fakeSnaps{},
	}
	cases := map[string]func(d *Deps){
		"settings":    func(d *Deps) { d.Settings = nil },
		"library":     func(d *Deps) { d.Library = nil },
		"downloads":   func(d *Deps) { d.Downloads = nil },
		"installs":    func(d *Deps) { d.Installs = nil },
		"relocations": func(d *Deps) { d.Relocations = nil },
		"updates":     func(d *Deps) { d.Updates = nil },
		"screenshots": func(d *Deps) { d.Screenshots = nil },
		"snapshots":   func(d *Deps) { d.Snapshots = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := good
			mutate(&d)
			if svc, err := NewService(d); err == nil || svc != nil {
				t.Fatalf("NewService = %v, %v; want an error", svc, err)
			}
		})
	}
	if _, err := NewService(good); err != nil {
		t.Fatalf("complete dependencies refused: %v", err)
	}
}

func TestScanBeforeStartupFails(t *testing.T) {
	svc, err := NewService(Deps{
		Settings: func() settings.Settings { return settings.Defaults() },
		Library:  &fakeLibrary{}, Downloads: &fakeDownloads{}, Installs: &fakeInstalls{},
		Relocations: &fakeRelocations{}, Updates: &fakeUpdates{}, Screenshots: &fakeShots{}, Snapshots: &fakeSnaps{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Scan(context.Background()); !errors.Is(err, errNotStarted) {
		t.Fatalf("Scan = %v, want errNotStarted", err)
	}
}

func TestScanGames(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "big", 300)
	r.addGame(t, "small", 20)
	r.lib.games = append(r.lib.games,
		library.Game{ID: "gone", Title: "Gone", InstallDir: filepath.Join(r.games, "gone")},
		library.Game{ID: "noDir", Title: "NoDir"},
	)
	r.lib.running = map[string]bool{"small": true}
	r.inst.busy["big"] = true

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]GameUsage{}
	for _, g := range rep.Games {
		by[g.ID] = g
	}
	if g := by["big"]; g.SizeBytes != 300 || g.SizeUnknown || !g.Busy || g.Running {
		t.Fatalf("big = %+v", g)
	}
	if g := by["small"]; g.SizeBytes != 20 || !g.Running || g.Busy {
		t.Fatalf("small = %+v", g)
	}
	if g := by["gone"]; !g.Missing || g.SizeUnknown || g.SizeBytes != 0 {
		t.Fatalf("gone = %+v, want missing and not unknown", g)
	}
	if g := by["noDir"]; !g.SizeUnknown {
		t.Fatalf("noDir = %+v, want an unknown size and not a zero", g)
	}
	if rep.Games[0].ID != "big" {
		t.Fatalf("games not sorted by size: %v", rep.Games)
	}
	if r.lib.updates["big"] != 300 || r.lib.updates["small"] != 20 {
		t.Fatalf("library not refreshed: %v", r.lib.updates)
	}
	if _, refreshed := r.lib.updates["gone"]; refreshed {
		t.Fatal("a missing folder must not overwrite the stored size")
	}
	if _, refreshed := r.lib.updates["noDir"]; refreshed {
		t.Fatal("an unknown size must not overwrite the stored size")
	}
	if rep.Totals.GamesBytes != 320 || !rep.Totals.UnknownSizes {
		t.Fatalf("totals = %+v", rep.Totals)
	}
	if cached := r.svc.Cached(); cached == nil || len(cached.Games) != len(rep.Games) {
		t.Fatalf("Cached = %+v", cached)
	}
}

func TestScanSkipsUninstalledGames(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "kept", 10)
	left := r.addGame(t, "left", 30)
	r.lib.games[1].Uninstalled = true
	r.lib.games = append(r.lib.games,
		library.Game{ID: "emptyDir", Title: "EmptyDir", Uninstalled: true},
		library.Game{ID: "goneDir", Title: "GoneDir", InstallDir: filepath.Join(r.games, "goneDir"), Uninstalled: true},
	)

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Games) != 1 || rep.Games[0].ID != "kept" {
		t.Fatalf("games = %+v, want only the installed one", rep.Games)
	}
	if rep.Totals.GamesBytes != 10 || rep.Totals.UnknownSizes {
		t.Fatalf("totals = %+v", rep.Totals)
	}
	if _, refreshed := r.lib.updates[left.ID]; refreshed {
		t.Fatal("an uninstalled game must not get a measured size")
	}
}

func TestScanReportsLibraryRefreshFailureAsWarning(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "a", 10)
	r.lib.updErr = errors.New("disk full")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Area != "games" || rep.Games[0].SizeBytes != 10 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestScanHonoursCancellation(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "a", 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.svc.Scan(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan = %v, want context.Canceled", err)
	}
	if r.svc.Cached() != nil {
		t.Fatal("a cancelled scan must not leave a report")
	}
}

func TestScanDownloads(t *testing.T) {
	r := newRig(t)
	mk := func(id string, status download.Status) download.Download {
		root := filepath.Join(r.downloads, id)
		writeBytes(t, filepath.Join(root, "data.bin"), 50)
		return download.Download{ID: id, Name: id, Destination: r.downloads, Status: status}.WithRoot(id)
	}
	free := mk("free", download.StatusCompleted)
	used := mk("used", download.StatusCompleted)
	seed := mk("seed", download.StatusCompleted)
	seed.Seeding = true
	running := mk("running", download.StatusDownloading)
	orphan := mk("orphan", download.StatusCompleted)
	flat := download.Download{ID: "flat", Destination: r.downloads, Status: download.StatusCompleted, Flat: true}.WithRoot("x")
	r.dl.items = []download.Download{free, used, seed, running, orphan, flat}
	r.inst.blocked["used"] = errDownloadBusy
	r.lib.games = []library.Game{
		{ID: "g1", Title: "G", SourceDownloadID: "free", InstallDir: filepath.Join(r.games, "g1")},
		{ID: "g2", Title: "Removed", SourceDownloadID: "orphan", InstallDir: filepath.Join(r.games, "g2"), Uninstalled: true},
	}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]DownloadUsage{}
	for _, d := range rep.Downloads {
		by[d.ID] = d
	}
	if _, ok := by["flat"]; ok {
		t.Fatal("a flat download has no folder of its own and must not be listed")
	}
	if d := by["free"]; d.Referenced || d.SizeBytes != 50 || !d.Installed || d.GameID != "g1" {
		t.Fatalf("free = %+v", d)
	}
	if d := by["orphan"]; d.Installed || d.GameID != "" {
		t.Fatalf("orphan = %+v, want no owner: its game was uninstalled", d)
	}
	for _, id := range []string{"used", "seed", "running"} {
		if d := by[id]; !d.Referenced || d.Reason == "" {
			t.Fatalf("%s = %+v, want referenced with a reason", id, d)
		}
	}
	if rep.Totals.ReclaimableBytes != 100 {
		t.Fatalf("reclaimable = %d, want only the unreferenced downloads", rep.Totals.ReclaimableBytes)
	}
}

var errDownloadBusy = errors.New("по этой загрузке идёт установка")

func TestScanLeftovers(t *testing.T) {
	r := newRig(t)
	okPartial := filepath.Join(r.games, "Done.partial")
	liveInstall := filepath.Join(r.games, "Live.partial")
	writeBytes(t, filepath.Join(okPartial, "a.bin"), 40)
	writeBytes(t, filepath.Join(liveInstall, "b.bin"), 10)
	writeBytes(t, filepath.Join(r.games, "notes.partial"), 5)
	r.inst.partial[okPartial] = true

	orphanPart := filepath.Join(r.downloads, "Old.part")
	ownedPart := filepath.Join(r.downloads, "Active.part")
	writeBytes(t, orphanPart, 70)
	writeBytes(t, ownedPart, 8)
	r.dl.owned[ownedPart] = true
	r.dl.partOK[orphanPart] = true
	foreignPart := filepath.Join(r.downloads, "browser.zip.part")
	writeBytes(t, foreignPart, 4)

	staging := filepath.Join(r.games, "Moved.staging")
	writeBytes(t, filepath.Join(staging, "c.bin"), 90)
	r.rel.list = []relocate.Staging{{Path: staging, Deletable: true}}

	game := r.addGame(t, "upd", 1)
	prev := game.InstallDir + ".previous"
	writeBytes(t, filepath.Join(prev, "old.bin"), 33)

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Leftover{}
	for _, l := range rep.Leftovers {
		by[l.Path] = l
	}
	want := []struct {
		path      string
		kind      string
		size      int64
		deletable bool
		code      string
	}{
		{okPartial, KindInstallPartial, 40, true, ""},
		{liveInstall, KindInstallPartial, 10, false, diskreason.OwnedByInstall},
		{orphanPart, KindDownloadPart, 70, true, ""},
		{ownedPart, KindDownloadPart, 8, false, diskreason.OwnedByDownload},
		{foreignPart, KindDownloadPart, 4, false, diskreason.ForeignPart},
		{staging, KindRelocateStaging, 90, true, ""},
		{prev, KindUpdatePrevious, 33, false, diskreason.UpdateOwned},
	}
	for _, w := range want {
		got, ok := by[w.path]
		if !ok || got.Kind != w.kind || got.SizeBytes != w.size || got.Deletable != w.deletable || got.ReasonCode != w.code {
			t.Fatalf("leftover %s = %+v (listed %v), want %+v", w.path, got, ok, w)
		}
		if !got.Deletable && got.Reason == "" {
			t.Fatalf("%s is refused without a reason", w.path)
		}
	}
	if _, ok := by[filepath.Join(r.games, "notes.partial")]; ok {
		t.Fatal("a plain file named .partial is not an install leftover")
	}
	if len(rep.Leftovers) != len(want) {
		t.Fatalf("leftovers = %+v, want %d", rep.Leftovers, len(want))
	}
}

func TestScanLeftoverSymlinkIsNotDeletable(t *testing.T) {
	r := newRig(t)
	target := t.TempDir()
	writeBytes(t, filepath.Join(target, "precious.bin"), 5)
	link := filepath.Join(r.games, "Link.partial")
	makeLink(t, link, target)
	r.inst.partial[link] = true

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Leftovers) != 0 {
		t.Fatalf("a link is not a directory entry we match: %+v", rep.Leftovers)
	}
}

func TestScanPartialFailuresBecomeWarnings(t *testing.T) {
	r := newRig(t)
	r.rel.err = errors.New("journal unreadable")
	r.shots.err = errors.New("no library")
	r.snaps.err = errors.New("disabled")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	areas := map[string]bool{}
	for _, w := range rep.Warnings {
		areas[w.Area] = true
	}
	if !areas["leftovers"] || !areas["screenshots"] || !areas["snapshots"] {
		t.Fatalf("warnings = %+v", rep.Warnings)
	}
}

func TestScanGroups(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "a", 1)
	r.shots.usage = []screenshots.Usage{{GameID: "a", Count: 2, SizeBytes: 20}, {GameID: "removed", Count: 1, SizeBytes: 5}}
	r.snaps.usage = []savebackup.Usage{{GameID: "a", Count: 3, SizeBytes: 300, SizeUnknown: true}}
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Screenshots) != 2 || rep.Screenshots[0].GameID != "a" || !rep.Screenshots[0].InLibrary || rep.Screenshots[0].Title != "Game a" {
		t.Fatalf("screenshots = %+v", rep.Screenshots)
	}
	if rep.Screenshots[1].InLibrary {
		t.Fatalf("a game that left the library must say so: %+v", rep.Screenshots[1])
	}
	if len(rep.Snapshots) != 1 || !rep.Snapshots[0].SizeUnknown || !rep.Totals.UnknownSizes {
		t.Fatalf("snapshots = %+v totals = %+v", rep.Snapshots, rep.Totals)
	}
	if rep.Totals.ScreenshotsBytes != 25 || rep.Totals.SnapshotsBytes != 300 {
		t.Fatalf("totals = %+v", rep.Totals)
	}
}
