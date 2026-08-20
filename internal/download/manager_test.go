package download

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeTorrent struct {
	mu          sync.Mutex
	size        int64
	done        int64
	downloading bool
	uploading   bool
	dropped     bool
	verified    bool
	priorities  []bool
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

func (f *fakeTorrent) verify(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = true
	return nil
}

func (f *fakeTorrent) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped = true
}

func (f *fakeTorrent) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = f.size
}

func newTestManager(t *testing.T, max int) *Manager {
	t.Helper()
	m := newManagerAt(t.TempDir(), nil)
	m.max = max
	return m
}

func (m *Manager) addTestDownload(id string) *fakeTorrent {
	eng := &fakeTorrent{size: 100}
	d := &Download{
		ID:          id,
		Name:        id,
		Type:        TypeTorrent,
		InfoHash:    id,
		Destination: "",
		Status:      StatusQueued,
		Total:       100,
		ETASeconds:  -1,
		Files:       []FileState{{Path: id, Size: 100, Selected: true}},
		AddedAt:     time.Now(),
	}
	m.mu.Lock()
	m.items = append(m.items, d)
	m.engines[id] = eng
	m.persistLocked()
	m.schedule()
	m.mu.Unlock()
	return eng
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
	engine.mu.Lock()
	dropped := engine.dropped
	engine.mu.Unlock()
	if !dropped {
		t.Fatal("engine not dropped")
	}
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
