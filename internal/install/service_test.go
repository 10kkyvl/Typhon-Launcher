package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/platform"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeDownloads struct {
	mu      sync.Mutex
	items   map[string]download.Download
	deleted []string
}

func newFakeDownloads() *fakeDownloads {
	return &fakeDownloads{items: map[string]download.Download{}}
}

func (f *fakeDownloads) add(id, name, destination string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[id] = download.Download{
		ID:          id,
		Name:        name,
		Destination: destination,
		Status:      download.StatusCompleted,
	}
}

func (f *fakeDownloads) Get(id string) (download.Download, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.items[id]
	if !ok {
		return download.Download{}, errors.New("загрузка не найдена")
	}
	return d, nil
}

func (f *fakeDownloads) DeleteData(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	delete(f.items, id)
	return nil
}

type fakeRegistrar struct {
	mu    sync.Mutex
	games []library.InstalledGame
}

func (f *fakeRegistrar) RegisterInstalled(g library.InstalledGame) (library.Game, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.games = append(f.games, g)
	return library.Game{ID: "game-1", Title: g.Title, Executable: g.Executable}, nil
}

func (f *fakeRegistrar) registered() []library.InstalledGame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]library.InstalledGame(nil), f.games...)
}

type fakeRunner struct {
	mu    sync.Mutex
	code  int
	err   error
	act   func(spec runSpec)
	specs []runSpec
}

func (f *fakeRunner) run(_ context.Context, spec runSpec) (int, error) {
	f.mu.Lock()
	f.specs = append(f.specs, spec)
	act, code, err := f.act, f.code, f.err
	f.mu.Unlock()
	if act != nil {
		act(spec)
	}
	return code, err
}

func (f *fakeRunner) calls() []runSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]runSpec(nil), f.specs...)
}

func newTestService(t *testing.T) (*Service, *fakeDownloads, *fakeRegistrar) {
	t.Helper()
	s := newServiceAt(t.TempDir(), nil)
	downloads := newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.downloads = downloads
	s.library = registrar
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return s, downloads, registrar
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (s *Service) waitStatus(t *testing.T, id string, want Status) Installation {
	t.Helper()
	var last Installation
	waitFor(t, string(want)+" status", func() bool {
		item, ok := s.snapshot(id)
		if !ok {
			return false
		}
		last = item
		return item.Status == want
	})
	return last
}

func portableSource(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	mkFile(t, filepath.Join(dir, name+".exe"), 256<<10)
	mkFile(t, filepath.Join(dir, "data", "content.pak"), 4096)
	return dir
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	completed := time.Now().Truncate(time.Second)
	items := []Installation{{
		ID:          "a",
		DownloadID:  "d1",
		Name:        "Game",
		Type:        TypePortable,
		Status:      StatusCompleted,
		Mode:        ModeCopy,
		Destination: filepath.Join(dir, "Game"),
		Executable:  filepath.Join(dir, "Game", "Game.exe"),
		Candidates:  []Candidate{{Path: "a.exe", Score: 42}},
		StartedAt:   completed,
		CompletedAt: &completed,
	}}

	st := newStore(dir)
	if err := st.save(items); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := st.load()
	if len(loaded) != 1 {
		t.Fatalf("loaded %d items", len(loaded))
	}
	got := loaded[0]
	if got.ID != "a" || got.Type != TypePortable || got.Mode != ModeCopy {
		t.Fatalf("got %+v", got)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Score != 42 {
		t.Fatalf("candidates = %+v", got.Candidates)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completed) {
		t.Fatalf("completedAt = %v", got.CompletedAt)
	}
}

func TestPortableInstallCompletes(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)

	info, err := s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Plan.Type != TypePortable {
		t.Fatalf("type = %s", info.Plan.Type)
	}
	if info.RequiredBytes <= 0 {
		t.Fatal("required bytes not estimated")
	}

	dest := filepath.Join(t.TempDir(), "Games", "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Executable != filepath.Join(dest, "Game.exe") {
		t.Fatalf("executable = %q", done.Executable)
	}
	if done.GameID != "game-1" {
		t.Fatalf("game id = %q", done.GameID)
	}
	if !exists(filepath.Join(dest, "data", "content.pak")) {
		t.Fatal("payload not installed")
	}
	if !exists(filepath.Join(source, "Game.exe")) {
		t.Fatal("copy mode removed the source")
	}
	if exists(dest + partialSuffix) {
		t.Fatal("partial dir left behind")
	}
	games := registrar.registered()
	if len(games) != 1 || games[0].InstallDir != dest || games[0].SourceDownloadID != "d1" {
		t.Fatalf("registered = %+v", games)
	}
}

func TestInstallKeepsDownloadProvenance(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	downloads.mu.Lock()
	d := downloads.items["d1"]
	d.Origin = download.Origin{ReleaseID: "rel-1", SourceID: "src-1", GameID: "canon-1"}
	downloads.items["d1"] = d
	downloads.mu.Unlock()

	dest := filepath.Join(t.TempDir(), "Games", "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if item.Origin.ReleaseID != "rel-1" {
		t.Fatalf("installation origin = %+v", item.Origin)
	}

	s.waitStatus(t, item.ID, StatusCompleted)
	games := registrar.registered()
	if len(games) != 1 {
		t.Fatalf("registered = %+v", games)
	}
	if games[0].ReleaseID != "rel-1" || games[0].SourceID != "src-1" || games[0].CanonicalGameID != "canon-1" {
		t.Fatalf("registered game = %+v, want provenance from the download", games[0])
	}
}

func TestPortableMoveIsForcedToCopyWhileSeeding(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	downloads.mu.Lock()
	d := downloads.items["d1"]
	d.Seeding = true
	downloads.items["d1"] = d
	downloads.mu.Unlock()

	dest := filepath.Join(t.TempDir(), "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if item.Mode != ModeCopy {
		t.Fatalf("mode = %q, want copy", item.Mode)
	}
	s.waitStatus(t, item.ID, StatusCompleted)
	if !exists(filepath.Join(source, "Game.exe")) {
		t.Fatal("seeding source was moved away")
	}
}

func TestArchiveInstallCompletes(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("g", 200<<10)
	writeZip(t, filepath.Join(dir, "game.zip"), []zipEntry{
		{name: "Game/Game.exe", data: []byte(payload)},
		{name: "Game/data/content.pak", data: []byte("assets")},
	})
	downloads.add("d1", "Game", root)

	info, err := s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Plan.Type != TypeArchiveZip {
		t.Fatalf("type = %s", info.Plan.Type)
	}

	dest := filepath.Join(t.TempDir(), "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Executable != filepath.Join(dest, "Game", "Game.exe") {
		t.Fatalf("executable = %q", done.Executable)
	}
	if exists(dest + partialSuffix) {
		t.Fatal("partial dir left behind")
	}
	if len(registrar.registered()) != 1 {
		t.Fatalf("registered = %+v", registrar.registered())
	}
}

func TestCancelDuringExtractionKeepsDestination(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	chunk := []byte(strings.Repeat("z", 512<<10))
	entries := make([]zipEntry, 0, 120)
	for i := 0; i < 120; i++ {
		entries = append(entries, zipEntry{name: "Game/part" + strconv.Itoa(i) + ".bin", data: chunk})
	}
	writeZip(t, filepath.Join(dir, "game.zip"), entries)
	downloads.add("d1", "Game", root)

	dest := filepath.Join(t.TempDir(), "Game")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, "extraction to start", func() bool {
		got, ok := s.snapshot(item.ID)
		return ok && got.Status == StatusExtracting && exists(dest+partialSuffix)
	})
	if err := s.Cancel(item.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	s.waitStatus(t, item.ID, StatusCancelled)
	waitFor(t, "partial dir removal", func() bool { return !exists(dest + partialSuffix) })
	if !exists(dest) {
		t.Fatal("pre-existing destination was removed")
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatalf("destination contents = %v (%v)", entries, err)
	}
}

func TestStartRefusesWithoutDiskSpace(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	s.freeSpace = func(string) (platform.StorageInfo, error) {
		return platform.StorageInfo{FreeBytes: 1024}, nil
	}

	dest := filepath.Join(t.TempDir(), "Game")
	if _, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy}); err == nil {
		t.Fatal("expected disk space error")
	} else if !strings.Contains(err.Error(), "недостаточно места") {
		t.Fatalf("error = %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("installations = %+v", got)
	}
	if exists(dest) {
		t.Fatal("destination created despite refusal")
	}
}

func TestStartRejectsUnknownType(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkText(t, filepath.Join(dir, "readme.txt"), "nothing useful")
	downloads.add("d1", "Game", root)

	if _, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game")}); !errors.Is(err, errUnknownType) {
		t.Fatalf("error = %v, want %v", err, errUnknownType)
	}
}

func TestTransientRecordsBecomeInterrupted(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "Game")
	partial := dest + partialSuffix
	mkFile(t, filepath.Join(partial, "chunk.bin"), 16)

	st := newStore(dir)
	if err := st.save([]Installation{
		{ID: "a", Name: "Game", Type: TypeArchiveZip, Status: StatusExtracting, Destination: dest},
		{ID: "b", Name: "Other", Type: TypeExeInstaller, Status: StatusWaitingForUser},
	}); err != nil {
		t.Fatal(err)
	}

	s := newServiceAt(dir, nil)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	defer s.ServiceShutdown()

	items := s.List()
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Status != StatusInterrupted || items[0].Error != interruptedMessage {
		t.Fatalf("first = %+v", items[0])
	}
	if items[1].Status != StatusWaitingForUser {
		t.Fatalf("second = %+v", items[1])
	}
	waitFor(t, "partial sweep", func() bool { return !exists(partial) })
}

func TestExeInstallerWaitsForConfirmation(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkFile(t, filepath.Join(dir, "setup.exe"), 4096)
	downloads.add("d1", "Game", root)

	programs := t.TempDir()
	installed := filepath.Join(programs, "MyGame")
	s.roots = []string{programs}
	s.runner = &fakeRunner{act: func(runSpec) {
		mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10)
		mkFile(t, filepath.Join(installed, "data", "content.pak"), 2048)
	}}

	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waiting := s.waitStatus(t, item.ID, StatusWaitingForUser)
	if len(waiting.Candidates) == 0 {
		t.Fatal("no candidates detected")
	}
	if waiting.Destination != installed {
		t.Fatalf("destination = %q, want %q", waiting.Destination, installed)
	}

	exe := filepath.Join(installed, "MyGame.exe")
	if err := s.ConfirmExecutable(item.ID, exe); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	done, _ := s.snapshot(item.ID)
	if done.Status != StatusCompleted || done.Executable != exe {
		t.Fatalf("installation = %+v", done)
	}
	games := registrar.registered()
	if len(games) != 1 || games[0].Executable != exe {
		t.Fatalf("registered = %+v", games)
	}
	calls := (s.runner.(*fakeRunner)).calls()
	if len(calls) != 1 || calls[0].Path != filepath.Join(dir, "setup.exe") {
		t.Fatalf("runner calls = %+v", calls)
	}
}

func TestExeInstallerFailsOnExitCode(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkFile(t, filepath.Join(dir, "setup.exe"), 4096)
	downloads.add("d1", "Game", root)

	s.roots = []string{t.TempDir()}
	s.runner = &fakeRunner{code: 1}

	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if failed.Error != errInstallerFail.Error() {
		t.Fatalf("error = %q", failed.Error)
	}
}

func TestCancelRefusedWhileInstallerRuns(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkFile(t, filepath.Join(dir, "setup.exe"), 4096)
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
	<-entered
	err = s.Cancel(item.ID)
	close(release)
	if !errors.Is(err, errExternalRuns) {
		t.Fatalf("error = %v, want %v", err, errExternalRuns)
	}
	s.waitStatus(t, item.ID, StatusWaitingForUser)
}

func TestRetryAfterFailure(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)

	dest := filepath.Join(t.TempDir(), "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)

	s.mu.Lock()
	stored := s.findLocked(item.ID)
	stored.Status = StatusFailed
	stored.Error = "boom"
	stored.Executable = ""
	s.mu.Unlock()
	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}

	if err := s.Retry(item.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	again := s.waitStatus(t, item.ID, StatusCompleted)
	if again.Error != "" {
		t.Fatalf("error = %q", again.Error)
	}
	if !exists(filepath.Join(source, "Game.exe")) {
		t.Fatal("source lost on retry")
	}
}

func TestDismissRemovesRecordAndPartial(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	partial := dest + partialSuffix
	mkFile(t, filepath.Join(partial, "chunk.bin"), 8)
	mkFile(t, filepath.Join(dest, "keep.txt"), 8)

	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID:          "a",
		Name:        "Game",
		Type:        TypeArchiveZip,
		Status:      StatusFailed,
		Destination: dest,
	})
	s.mu.Unlock()

	if err := s.Dismiss("a"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("items = %+v", got)
	}
	waitFor(t, "partial removal", func() bool { return !exists(partial) })
	if !exists(filepath.Join(dest, "keep.txt")) {
		t.Fatal("destination contents removed")
	}
}

func TestDismissRejectsActiveInstall(t *testing.T) {
	s, _, _ := newTestService(t)
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "a", Name: "Game", Status: StatusWaitingForUser})
	s.mu.Unlock()
	if err := s.Dismiss("a"); !errors.Is(err, errUnavailable) {
		t.Fatalf("error = %v, want %v", err, errUnavailable)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		`Game: The/Best*Edition`: "Game The Best Edition",
		"  spaced   out  ":       "spaced out",
		`???`:                    "Game",
		"trailing.":              "trailing",
	}
	for input, want := range cases {
		if got := sanitizeName(input); got != want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProposeDestinationAvoidsCollision(t *testing.T) {
	s := newServiceAt(t.TempDir(), nil)
	games := t.TempDir()
	mkFile(t, filepath.Join(games, "Game", "taken.txt"), 4)

	got := s.proposeDestination(games, "Game")
	if got != filepath.Join(games, "Game (2)") {
		t.Fatalf("destination = %q", got)
	}
}
