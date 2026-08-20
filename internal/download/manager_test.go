package download

import (
	"context"
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
	priorities   []bool
	entered      chan struct{}
	gate         chan struct{}
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
	m := newManagerAt(t.TempDir(), nil)
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
	m.sample(time.Now())

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
	m.sample(time.Now())

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
	m.sample(time.Now())

	if got := m.statusOf(t, "a"); got != StatusCompleted {
		t.Fatalf("status = %s, want %s", got, StatusCompleted)
	}
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

	reloaded := newManagerAt(m.store.dir, nil)
	reloaded.mu.Lock()
	reloaded.loadLocked()
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

func TestCompletionNotifiesInstaller(t *testing.T) {
	m := newTestManager(t, 2)
	seen := make(chan Download, 1)
	m.SetOnCompleted(func(d Download) { seen <- d })

	eng := m.addTestDownload("a")
	eng.mu.Lock()
	eng.done = 100
	eng.mu.Unlock()
	m.sample(time.Now())

	select {
	case d := <-seen:
		if d.ID != "a" || d.Status != StatusCompleted {
			t.Fatalf("notified with %+v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completion callback not invoked")
	}
}
