package download

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/settings"
)

type fakeTorrent struct {
	mu           sync.Mutex
	size         int64
	done         int64
	downloading  bool
	uploading    bool
	dropped      bool
	droppedEarly bool
	verified     bool
	verifying    bool
	hashed       bool
	priorities   []bool
	entered      chan struct{}
	gate         chan struct{}
	paths        []string
}

func (f *fakeTorrent) setPriorities(selected []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.priorities = append([]bool(nil), selected...)
}

func (f *fakeTorrent) allowDownload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloading = true
}

func (f *fakeTorrent) disallowDownload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloading = false
}

func (f *fakeTorrent) allowUpload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploading = true
}

func (f *fakeTorrent) disallowUpload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploading = false
}

func (f *fakeTorrent) fileBytes() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []int64{f.done}
}

func (f *fakeTorrent) filesHashed() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []bool{f.hashed}
}

func (f *fakeTorrent) filePaths(_ string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.paths
}

func (f *fakeTorrent) stats() engineStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return engineStats{downloaded: f.done, seeders: 1, peers: 3}
}

func (f *fakeTorrent) verify(ctx context.Context) error {
	f.mu.Lock()
	entered, gate := f.entered, f.gate
	f.entered = nil
	f.verifying = true
	f.mu.Unlock()

	if entered != nil {
		close(entered)
	}
	var err error
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.verifying = false
	f.verified = err == nil
	return err
}

func (f *fakeTorrent) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.verifying {
		f.droppedEarly = true
	}
	f.dropped = true
}

func (f *fakeTorrent) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = f.size
	f.hashed = true
}

func (f *fakeTorrent) blockVerify() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entered = make(chan struct{})
	f.gate = make(chan struct{})
	return f.entered
}

func (f *fakeTorrent) wasDropped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dropped
}

func (f *fakeTorrent) wasDroppedDuringVerify() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.droppedEarly
}

func (f *fakeTorrent) isUploading() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploading
}

func newTestManager(t *testing.T, max int) *Manager {
	t.Helper()
	m := mustManagerAt(t, t.TempDir())
	m.max = max
	return m
}

func (m *Manager) addTestItem(id string, status Status) *Download {
	d := &Download{
		ID:         id,
		Name:       id,
		Type:       TypeTorrent,
		InfoHash:   id,
		Status:     status,
		Total:      100,
		ETASeconds: -1,
		Files:      []FileState{{Path: id, Size: 100, Selected: true}},
		AddedAt:    time.Now(),
	}
	m.mu.Lock()
	m.items = append(m.items, d)
	m.persistLocked()
	m.mu.Unlock()
	return d
}

func (m *Manager) addTestDownload(id string) *fakeTorrent {
	eng := &fakeTorrent{size: 100}
	m.addTestItem(id, StatusQueued)
	m.mu.Lock()
	m.engines[id] = eng
	m.schedule()
	m.mu.Unlock()
	return eng
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (m *Manager) statusOf(t *testing.T, id string) Status {
	t.Helper()
	d, err := m.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return d.Status
}

func (m *Manager) order() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.items))
	for _, d := range m.items {
		ids = append(ids, d.ID)
	}
	return ids
}

func assertStatuses(t *testing.T, m *Manager, want map[string]Status) {
	t.Helper()
	for id, status := range want {
		if got := m.statusOf(t, id); got != status {
			t.Fatalf("%s status = %s, want %s", id, got, status)
		}
	}
}

func mustManagerAt(t testing.TB, dir string) *Manager {
	t.Helper()
	m, err := newManagerAt(dir, nil)
	if err != nil {
		t.Fatalf("new download manager at %s: %v", dir, err)
	}
	t.Cleanup(func() {
		// ServiceShutdown already closed it in tests that call it themselves
		// and nils the field, so this only fires for tests that never start
		// the manager.
		if m.pieceCompletion != nil {
			if err := m.pieceCompletion.Close(); err != nil {
				t.Logf("close piece completion: %v", err)
			}
		}
	})
	withTestContext(t, m)
	return m
}

func withTestContext(t testing.TB, m *Manager) {
	t.Helper()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	t.Cleanup(m.cancel)
}

func fakeFilePath(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "download.bin")
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write fake file: %v", err)
	}
	return path
}

func TestQueueRespectsMaxActive(t *testing.T) {
	m := newTestManager(t, 2)
	for _, id := range []string{"a", "b", "c", "d"} {
		m.addTestDownload(id)
	}
	assertStatuses(t, m, map[string]Status{
		"a": StatusDownloading,
		"b": StatusDownloading,
		"c": StatusQueued,
		"d": StatusQueued,
	})
}

func TestCompletionPromotesNext(t *testing.T) {
	m := newTestManager(t, 2)
	first := m.addTestDownload("a")
	for _, id := range []string{"b", "c", "d"} {
		m.addTestDownload(id)
	}

	first.finish()
	first.mu.Lock()
	first.paths = []string{fakeFilePath(t, first.size)}
	first.mu.Unlock()
	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
	assertStatuses(t, m, map[string]Status{
		"a": StatusCompleted,
		"b": StatusDownloading,
		"c": StatusDownloading,
		"d": StatusQueued,
	})
	done, _ := m.Get("a")
	if done.CompletedAt == nil || done.Progress != 1 {
		t.Fatalf("completed download = %+v", done)
	}
	if done.Seeding {
		t.Fatal("seeding enabled without the setting")
	}
}

func TestPauseFreesSlot(t *testing.T) {
	m := newTestManager(t, 2)
	engine := m.addTestDownload("a")
	for _, id := range []string{"b", "c"} {
		m.addTestDownload(id)
	}

	if err := m.Pause("a"); err != nil {
		t.Fatal(err)
	}
	assertStatuses(t, m, map[string]Status{
		"a": StatusPaused,
		"b": StatusDownloading,
		"c": StatusDownloading,
	})
	engine.mu.Lock()
	paused := !engine.downloading && !engine.uploading
	engine.mu.Unlock()
	if !paused {
		t.Fatal("engine still allowed to transfer")
	}
}

func TestResumeWhenFullStaysQueued(t *testing.T) {
	m := newTestManager(t, 2)
	for _, id := range []string{"a", "b", "c"} {
		m.addTestDownload(id)
	}
	if err := m.Pause("a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Resume("a"); err != nil {
		t.Fatal(err)
	}
	assertStatuses(t, m, map[string]Status{"a": StatusQueued})
}

func TestForceStartExceedsLimit(t *testing.T) {
	m := newTestManager(t, 2)
	for _, id := range []string{"a", "b", "c"} {
		m.addTestDownload(id)
	}
	if err := m.ForceStart("c"); err != nil {
		t.Fatal(err)
	}
	assertStatuses(t, m, map[string]Status{
		"a": StatusDownloading,
		"b": StatusDownloading,
		"c": StatusDownloading,
	})
}

func TestPauseCompletedIsRejected(t *testing.T) {
	m := newTestManager(t, 1)
	engine := m.addTestDownload("a")
	engine.finish()
	engine.mu.Lock()
	engine.paths = []string{fakeFilePath(t, engine.size)}
	engine.mu.Unlock()
	m.sample(context.Background(), time.Now())
	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })

	if err := m.Pause("a"); err != errUnavailable {
		t.Fatalf("pause completed = %v, want %v", err, errUnavailable)
	}
	if err := m.Resume("a"); err != errUnavailable {
		t.Fatalf("resume completed = %v, want %v", err, errUnavailable)
	}
}

func TestUnknownDownload(t *testing.T) {
	m := newTestManager(t, 2)
	if err := m.Pause("nope"); err != errNotFound {
		t.Fatalf("pause = %v, want %v", err, errNotFound)
	}
	if _, err := m.Get("nope"); err != errNotFound {
		t.Fatalf("get = %v, want %v", err, errNotFound)
	}
}

func TestMoveWithinQueue(t *testing.T) {
	m := newTestManager(t, 1)
	for _, id := range []string{"a", "b", "c", "d"} {
		m.addTestDownload(id)
	}

	if err := m.MoveUp("d"); err != nil {
		t.Fatal(err)
	}
	if got := m.order(); got[2] != "d" || got[3] != "c" {
		t.Fatalf("order = %v", got)
	}
	if err := m.MoveDown("d"); err != nil {
		t.Fatal(err)
	}
	if got := m.order(); got[2] != "c" || got[3] != "d" {
		t.Fatalf("order = %v", got)
	}
}

func TestMoveBounds(t *testing.T) {
	m := newTestManager(t, 1)
	for _, id := range []string{"a", "b", "c"} {
		m.addTestDownload(id)
	}

	if err := m.MoveUp("b"); err != nil {
		t.Fatal(err)
	}
	if got := m.order(); got[1] != "b" {
		t.Fatalf("queued item moved above active one: %v", got)
	}
	if err := m.MoveDown("c"); err != nil {
		t.Fatal(err)
	}
	if got := m.order(); got[2] != "c" {
		t.Fatalf("order = %v", got)
	}
	if err := m.MoveUp("a"); err != errUnavailable {
		t.Fatalf("move active = %v, want %v", err, errUnavailable)
	}
}

func TestCancelDropsAndRemoves(t *testing.T) {
	m := newTestManager(t, 2)
	engine := m.addTestDownload("a")
	m.addTestDownload("b")
	m.addTestDownload("c")

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "torrent to be dropped", engine.wasDropped)
	if _, err := m.Get("a"); err != errNotFound {
		t.Fatalf("get after cancel = %v", err)
	}
	assertStatuses(t, m, map[string]Status{"b": StatusDownloading, "c": StatusDownloading})
}

func TestRemoveKeepsFiles(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestDownload("a")
	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("list = %v", m.List())
	}
}

func TestListIsSnapshot(t *testing.T) {
	m := newTestManager(t, 1)
	m.addTestDownload("a")
	list := m.List()
	list[0].Files[0].Selected = false
	if !m.List()[0].Files[0].Selected {
		t.Fatal("snapshot shares file state")
	}
}

func TestCancelDuringVerifyDoesNotDeadlock(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	eng := &fakeTorrent{size: 100}
	entered := eng.blockVerify()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobCtx, started := m.beginJob(ctx, "a")
	if !started {
		t.Fatal("beginJob refused to start")
	}

	settled := make(chan struct{})
	go func() {
		defer close(settled)
		defer m.endJob("a")
		m.settleRestored(jobCtx, restoreJob{id: "a"}, eng, nil)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("verify never started")
	}
	if got := m.statusOf(t, "a"); got != StatusVerifying {
		t.Fatalf("status = %s, want %s", got, StatusVerifying)
	}

	cancelled := make(chan error, 1)
	go func() { cancelled <- m.Cancel("a") }()
	select {
	case err := <-cancelled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel blocked while a verify was in flight")
	}

	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("verify goroutine never exited")
	}

	waitUntil(t, "torrent to be dropped", eng.wasDropped)
	if eng.wasDroppedDuringVerify() {
		t.Fatal("torrent dropped while verify was still running")
	}
	if _, err := m.Get("a"); err != errNotFound {
		t.Fatalf("get after cancel = %v, want %v", err, errNotFound)
	}
	if len(m.List()) != 0 {
		t.Fatal("manager still reports the cancelled download")
	}
}

func TestRemoveDuringVerifyWaitsForVerify(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	eng := &fakeTorrent{size: 100}
	entered := eng.blockVerify()

	jobCtx, started := m.beginJob(context.Background(), "a")
	if !started {
		t.Fatal("beginJob refused to start")
	}
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		defer m.endJob("a")
		m.settleRestored(jobCtx, restoreJob{id: "a"}, eng, nil)
	}()
	<-entered

	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	<-settled
	waitUntil(t, "torrent to be dropped", eng.wasDropped)
	if eng.wasDroppedDuringVerify() {
		t.Fatal("torrent dropped while verify was still running")
	}
}

func TestVerifyQueuesDownloadWhenFinished(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	eng := &fakeTorrent{size: 100}

	jobCtx, started := m.beginJob(context.Background(), "a")
	if !started {
		t.Fatal("beginJob refused to start")
	}
	m.settleRestored(jobCtx, restoreJob{id: "a"}, eng, nil)
	m.endJob("a")

	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %s, want %s", got, StatusDownloading)
	}
}

func TestResumeWithoutEngineReportsRestoreFailure(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusFailed)
	m.mu.Lock()
	d.Source = `D:\gone.torrent`
	d.Error = restoreFailedMessage
	m.mu.Unlock()

	if err := m.Resume("a"); err != errNoRestore {
		t.Fatalf("resume = %v, want %v", err, errNoRestore)
	}
	if got := m.statusOf(t, "a"); got != StatusFailed {
		t.Fatalf("status = %s, want %s", got, StatusFailed)
	}
	if err := m.ForceStart("a"); err != errNoRestore {
		t.Fatalf("force start = %v, want %v", err, errNoRestore)
	}
	if got := m.statusOf(t, "a"); got != StatusFailed {
		t.Fatalf("status = %s, want %s", got, StatusFailed)
	}
}

func TestResumeWithoutEngineNeedsClientToReattach(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusFailed)
	m.mu.Lock()
	d.Source = "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)
	m.mu.Unlock()

	if err := m.Resume("a"); err != errNoClient {
		t.Fatalf("resume = %v, want %v", err, errNoClient)
	}
	if got := m.statusOf(t, "a"); got != StatusFailed {
		t.Fatalf("status = %s, want %s", got, StatusFailed)
	}
}

func TestCompletionWithoutSeedingDropsEngine(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()
	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
	if got, _ := m.Get("a"); got.Seeding {
		t.Fatal("seeding reported with seed-after-download off")
	}
	waitUntil(t, "engine drop", eng.wasDropped)
	m.mu.Lock()
	_, attached := m.engines["a"]
	m.mu.Unlock()
	if attached {
		t.Fatal("engine still attached after completion")
	}
}

func TestSeedToggleWithEngine(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusCompleted)
	eng := &fakeTorrent{size: 100, done: 100, uploading: true}
	m.mu.Lock()
	d.Seeding = true
	m.engines["a"] = eng
	m.mu.Unlock()

	cfg := settings.Defaults()
	cfg.SeedAfterDownload = false
	m.applySettings(cfg)
	if got, _ := m.Get("a"); got.Seeding {
		t.Fatal("seeding still reported")
	}
	if eng.isUploading() {
		t.Fatal("upload still allowed")
	}

	cfg.SeedAfterDownload = true
	m.applySettings(cfg)
	if got, _ := m.Get("a"); !got.Seeding {
		t.Fatal("seeding not enabled")
	}
	if !eng.isUploading() {
		t.Fatal("upload not allowed")
	}
}

func TestSeedToggleWithoutEngineDoesNotClaimSeeding(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	d.Seeding = true
	d.Source = `D:\gone.torrent`
	m.mu.Unlock()

	cfg := settings.Defaults()
	cfg.SeedAfterDownload = true
	m.applySettings(cfg)

	if got, _ := m.Get("a"); got.Seeding {
		t.Fatal("seeding reported without a live torrent")
	}
}

func TestFailWithoutClientSurfacesError(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	m.addTestItem("b", StatusPaused)
	done := m.addTestItem("c", StatusCompleted)
	m.mu.Lock()
	done.Seeding = true
	m.mu.Unlock()

	m.failWithoutClient()

	queued, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != StatusFailed || queued.Error != errNoClient.Error() {
		t.Fatalf("queued download = %s / %q", queued.Status, queued.Error)
	}
	if paused, _ := m.Get("b"); paused.Status != StatusPaused {
		t.Fatalf("paused download = %s, want %s", paused.Status, StatusPaused)
	}
	got, _ := m.Get("c")
	if got.Status != StatusCompleted || got.Seeding {
		t.Fatalf("completed download = %s seeding=%v", got.Status, got.Seeding)
	}
}

func TestCancelDeletesDataOutsideTheLock(t *testing.T) {
	dest := t.TempDir()
	root := filepath.Join(dest, "a")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chunk.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	m.mu.Lock()
	m.findLocked("a").Destination = dest
	m.mu.Unlock()

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "download data to be deleted", func() bool {
		_, err := os.Stat(root)
		return os.IsNotExist(err)
	})
	waitUntil(t, "torrent to be dropped", eng.wasDropped)
}

func TestRemoveKeepsDataOnDisk(t *testing.T) {
	dest := t.TempDir()
	root := filepath.Join(dest, "a")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	m.mu.Lock()
	m.findLocked("a").Destination = dest
	m.mu.Unlock()

	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "torrent to be dropped", eng.wasDropped)
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("data removed by Remove: %v", err)
	}
}

func TestDiscardMetainfoKeepsFilesStillInUse(t *testing.T) {
	m := newTestManager(t, 2)
	writeTestMetainfo(t, m.store, "a")
	writeTestMetainfo(t, m.store, "orphan")
	m.addTestItem("a", StatusQueued)

	m.discardMetainfo("a")
	m.discardMetainfo("orphan")

	if !m.store.hasMetainfo("a") {
		t.Fatal("metainfo of a live download was removed")
	}
	if m.store.hasMetainfo("orphan") {
		t.Fatal("orphaned metainfo was kept")
	}
}

func TestDiscardMetadataRemovesUnusedMetainfo(t *testing.T) {
	m := newTestManager(t, 2)
	writeTestMetainfo(t, m.store, "abc")

	m.DiscardMetadata("abc")

	if m.store.hasMetainfo("abc") {
		t.Fatal("metainfo kept after discard")
	}
}

func TestPersistedStateReloads(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestDownload("a")
	m.addTestDownload("b")

	reloaded := mustManagerAt(t, m.store.dir)
	reloaded.mu.Lock()
	if err := reloaded.loadLocked(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	reloaded.mu.Unlock()

	items := reloaded.List()
	if len(items) != 2 {
		t.Fatalf("reloaded %d items, want 2", len(items))
	}
	if items[0].Status != StatusQueued || items[1].Status != StatusQueued {
		t.Fatalf("statuses = %s %s, want queued", items[0].Status, items[1].Status)
	}
}

func TestDeleteDataRefusesWhileSeeding(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	m.findLocked("a").Seeding = true
	m.mu.Unlock()

	if err := m.DeleteData("a"); err != errSeeding {
		t.Fatalf("error = %v, want %v", err, errSeeding)
	}
	if _, err := m.Get("a"); err != nil {
		t.Fatalf("download dropped: %v", err)
	}
}

func TestDeleteDataRefusesUnfinishedDownload(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	if err := m.DeleteData("a"); err != errUnavailable {
		t.Fatalf("error = %v, want %v", err, errUnavailable)
	}
	if err := m.DeleteData("missing"); err != errNotFound {
		t.Fatalf("error = %v, want %v", err, errNotFound)
	}
}

func TestDeleteDataRemovesRecordAndFiles(t *testing.T) {
	dest := t.TempDir()
	root := filepath.Join(dest, "a")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "game.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	m.findLocked("a").Destination = dest
	m.mu.Unlock()

	if err := m.DeleteData("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("a"); err == nil {
		t.Fatal("record kept after delete")
	}
	waitUntil(t, "download data to be deleted", func() bool {
		_, err := os.Stat(root)
		return os.IsNotExist(err)
	})
}

func TestBytesWithoutHashKeepsDownloading(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	final := filepath.Join(t.TempDir(), "download.bin")
	if err := os.WriteFile(final+PartFileSuffix, make([]byte, eng.size), 0o644); err != nil {
		t.Fatalf("write part file: %v", err)
	}
	eng.mu.Lock()
	eng.done = eng.size
	eng.paths = []string{final}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %s, want %s", got, StatusDownloading)
	}
	d, err := m.Get("a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if d.Error != "" {
		t.Fatalf("error = %q, want empty", d.Error)
	}

	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()
	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
}

func TestCompletionNotifiesInstaller(t *testing.T) {
	m := newTestManager(t, 2)
	seen := make(chan Download, 1)
	m.SetOnCompleted(func(d Download) { seen <- d })

	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()
	m.sample(context.Background(), time.Now())

	select {
	case d := <-seen:
		if d.ID != "a" || d.Status != StatusCompleted {
			t.Fatalf("notified with %+v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completion callback not invoked")
	}
}

func newManagerWithSettings(t *testing.T, cfg settings.Settings) (*Manager, *settings.Service) {
	t.Helper()
	dir := t.TempDir()
	svc, err := settings.NewServiceAt(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	m, err := newManagerAt(dir, svc)
	if err != nil {
		t.Fatalf("new download manager at %s: %v", dir, err)
	}
	withTestContext(t, m)
	m.max = 2
	return m, svc
}

func TestUploadSettingCombinations(t *testing.T) {
	cases := []struct {
		name            string
		uploadWhile     bool
		seedAfter       bool
		duringDownload  bool
		afterCompletion bool
	}{
		{"both off", false, false, false, false},
		{"seed after only", false, true, false, true},
		{"upload while only", true, false, true, false},
		{"both on", true, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := settings.Defaults()
			cfg.UploadWhileDownloading = c.uploadWhile
			cfg.SeedAfterDownload = c.seedAfter
			m, _ := newManagerWithSettings(t, cfg)

			eng := m.addTestDownload("a")
			if got := m.statusOf(t, "a"); got != StatusDownloading {
				t.Fatalf("status = %s, want %s", got, StatusDownloading)
			}
			if got := eng.isUploading(); got != c.duringDownload {
				t.Fatalf("uploading during download = %v, want %v", got, c.duringDownload)
			}

			eng.finish()
			eng.mu.Lock()
			eng.paths = []string{fakeFilePath(t, eng.size)}
			eng.mu.Unlock()
			m.sample(context.Background(), time.Now())
			waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
			if got := eng.isUploading(); got != c.afterCompletion {
				t.Fatalf("uploading after completion = %v, want %v", got, c.afterCompletion)
			}
			done, err := m.Get("a")
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if done.Seeding != c.seedAfter {
				t.Fatalf("seeding = %v, want %v", done.Seeding, c.seedAfter)
			}
		})
	}
}

func TestStartKeepsUploadOffByDefault(t *testing.T) {
	m, _ := newManagerWithSettings(t, settings.Defaults())
	eng := m.addTestDownload("a")
	if eng.isUploading() {
		t.Fatal("upload allowed with upload-while-downloading off")
	}
}

func TestResumeReappliesUploadSetting(t *testing.T) {
	cfg := settings.Defaults()
	cfg.UploadWhileDownloading = true
	m, svc := newManagerWithSettings(t, cfg)

	eng := m.addTestDownload("a")
	if !eng.isUploading() {
		t.Fatal("upload not allowed with the setting on")
	}
	if err := m.Pause("a"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if eng.isUploading() {
		t.Fatal("upload still allowed while paused")
	}

	cfg.UploadWhileDownloading = false
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := m.Resume("a"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %s, want %s", got, StatusDownloading)
	}
	if eng.isUploading() {
		t.Fatal("resume re-enabled upload with the setting off")
	}
}

func TestUploadSettingAppliesToActiveDownload(t *testing.T) {
	cfg := settings.Defaults()
	m, _ := newManagerWithSettings(t, cfg)
	eng := m.addTestDownload("a")
	if eng.isUploading() {
		t.Fatal("upload allowed with the setting off")
	}

	cfg.UploadWhileDownloading = true
	m.applySettings(cfg)
	if !eng.isUploading() {
		t.Fatal("upload not allowed after enabling the setting")
	}

	cfg.UploadWhileDownloading = false
	m.applySettings(cfg)
	if eng.isUploading() {
		t.Fatal("upload still allowed after disabling the setting")
	}
}

func TestUploadWhileDownloadingDoesNotSeedCompleted(t *testing.T) {
	cfg := settings.Defaults()
	cfg.UploadWhileDownloading = true
	cfg.SeedAfterDownload = false
	m, _ := newManagerWithSettings(t, cfg)

	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()
	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
	if eng.isUploading() {
		t.Fatal("completed download keeps uploading with seed-after-download off")
	}
	waitUntil(t, "engine drop", eng.wasDropped)
}

func TestCompletionFailsWhenFileMissingFromDisk(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	eng.mu.Lock()
	eng.done = eng.size
	eng.hashed = true
	eng.paths = []string{filepath.Join(t.TempDir(), "missing.bin")}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to fail", func() bool { return m.statusOf(t, "a") == StatusFailed })
	got, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == "" {
		t.Fatal("failed download has no error message")
	}
}

func TestCompletionFailsWhenFileTruncatedOnDisk(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	path := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(path, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	eng.mu.Lock()
	eng.done = eng.size
	eng.hashed = true
	eng.paths = []string{path}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to fail", func() bool { return m.statusOf(t, "a") == StatusFailed })
	got, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == "" {
		t.Fatal("failed download has no error message")
	}
}

func TestCompletionSucceedsWhenFileMatchesOnDisk(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })
	got, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
}

type logSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func captureLogs(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return sink
}

func TestLogsHideInfoHashAndSource(t *testing.T) {
	const infoHash = "a748597437835a2fd0d2e06f8edd86fee316a84d"
	m, _ := newManagerWithSettings(t, settings.Defaults())
	d := m.addTestItem("a", StatusPaused)
	m.mu.Lock()
	d.InfoHash = infoHash
	d.Source = `D:\Downloads\Startup Panic.torrent`
	m.mu.Unlock()

	sink := captureLogs(t)
	if err := m.Resume("a"); !errors.Is(err, errNoRestore) {
		t.Fatalf("resume = %v, want %v", err, errNoRestore)
	}

	logged := sink.text()
	if !strings.Contains(logged, "download_id=a") {
		t.Fatalf("log has no download id: %q", logged)
	}
	for _, leak := range []string{infoHash, "Startup Panic", ".torrent"} {
		if strings.Contains(logged, leak) {
			t.Fatalf("log leaks %q: %q", leak, logged)
		}
	}
}

func TestCompletionWaitsWhileAnotherJobHoldsDownload(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()

	_, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.jobs["a"] = &jobState{cancel: cancel, done: make(chan struct{})}
	m.mu.Unlock()

	m.sample(context.Background(), time.Now())

	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %q, want %q while another job holds the download", got, StatusDownloading)
	}

	m.mu.Lock()
	delete(m.jobs, "a")
	m.mu.Unlock()
	cancel()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete once the job is released", func() bool {
		return m.statusOf(t, "a") == StatusCompleted
	})
}
