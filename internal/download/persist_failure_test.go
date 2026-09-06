package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// breakStore repoints the manager's store at a path that can never be
// created (a regular file sits where a directory needs to go), so any
// subsequent persistLocked deterministically fails the same way on every OS,
// mirroring internal/history's own TestRecordRollsBackOnPersistFailure.
func breakStore(t *testing.T, m *Manager) {
	t.Helper()
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.store.dir = filepath.Join(blocker, "downloads")
	m.mu.Unlock()
}

// breakDownloadsFile fails only downloads.json's write (a directory sits at
// that exact path, so storage.WriteAtomic's rename fails) while leaving
// store.dir and its torrents/ subdirectory intact, so a metainfo lookup
// saved before this call still resolves. Use this instead of breakStore
// whenever the code under test also needs to read cached metainfo.
func breakDownloadsFile(t *testing.T, m *Manager) {
	t.Helper()
	m.mu.Lock()
	dir := m.store.dir
	m.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(dir, "downloads.json"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (m *Manager) degradedStatus() degradedStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.degraded
}

func TestPauseRollsBackOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	if !eng.downloading {
		t.Fatal("setup: engine not downloading")
	}
	breakStore(t, m)

	err := m.Pause("a")
	if err == nil {
		t.Fatal("Pause must return the persist failure")
	}
	if got := m.statusOf(t, "a"); got != StatusDownloading {
		t.Fatalf("status = %s, want %s (rolled back)", got, StatusDownloading)
	}
	if !eng.downloading {
		t.Fatal("engine was gated off despite the persist failure; Pause must not touch it before a successful persist")
	}
	if st := m.degradedStatus(); !st.Degraded || st.Message == "" {
		t.Fatalf("degraded status = %+v, want degraded with a message", st)
	}
}

func TestResumeRollsBackOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusPaused)
	eng := &fakeTorrent{size: 100}
	m.mu.Lock()
	m.engines["a"] = eng
	m.mu.Unlock()
	breakStore(t, m)

	err := m.Resume("a")
	if err == nil {
		t.Fatal("Resume must return the persist failure")
	}
	if d.Status != StatusPaused {
		t.Fatalf("status = %s, want %s (rolled back)", d.Status, StatusPaused)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded status = %+v, want degraded", st)
	}
}

func TestForceStartRollsBackOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusQueued)
	eng := &fakeTorrent{size: 100}
	m.mu.Lock()
	m.engines["a"] = eng
	m.mu.Unlock()
	breakStore(t, m)

	err := m.ForceStart("a")
	if err == nil {
		t.Fatal("ForceStart must return the persist failure")
	}
	if d.Status != StatusQueued {
		t.Fatalf("status = %s, want %s (rolled back)", d.Status, StatusQueued)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded status = %+v, want degraded", st)
	}
}

func TestMoveRollsBackOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	m.addTestItem("b", StatusQueued)
	before := m.order()
	breakStore(t, m)

	if err := m.MoveDown("a"); err == nil {
		t.Fatal("MoveDown must return the persist failure")
	}
	if got := m.order(); got[0] != before[0] || got[1] != before[1] {
		t.Fatalf("order = %v, want unchanged %v", got, before)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded status = %+v, want degraded", st)
	}
}

func TestDeleteDataRollsBackOnPersistFailureWithoutTearingDown(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	eng := &fakeTorrent{size: 100}
	m.mu.Lock()
	m.engines["a"] = eng
	m.mu.Unlock()
	breakStore(t, m)

	err := m.DeleteData("a")
	if err == nil {
		t.Fatal("DeleteData must return the persist failure")
	}
	if got := m.statusOf(t, "a"); got != StatusCompleted {
		t.Fatalf("status = %s, want %s (item must still be tracked)", got, StatusCompleted)
	}
	m.mu.Lock()
	_, stillAttached := m.engines["a"]
	m.mu.Unlock()
	if !stillAttached {
		t.Fatal("engine was detached despite the failed removal; nothing must be torn down before the record is durably removed")
	}
	if eng.wasDropped() {
		t.Fatal("engine was dropped despite the failed removal")
	}
}

func TestCancelRollsBackOnPersistFailureWithoutTearingDown(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	breakStore(t, m)

	if err := m.Cancel("a"); err == nil {
		t.Fatal("Cancel must return the persist failure")
	}
	if got := m.statusOf(t, "a"); got != StatusQueued {
		t.Fatalf("status = %s, want %s (item must still be tracked)", got, StatusQueued)
	}
}

func TestAddTaskRollsBackOnPersistFailure(t *testing.T) {
	mi, _ := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()

	m := newTestManager(t, 5)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}
	breakDownloadsFile(t, m)

	_, err := m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: t.TempDir()})
	if err == nil {
		t.Fatal("AddTask must return the persist failure")
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("items tracked by manager = %d, want 0", got)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the real client = %d, want 0 (the failed add must drop its torrent)", got)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded status = %+v, want degraded", st)
	}
}

// TestMarkFailedKeepsFailedStatusOnPersistFailure documents a deliberate
// asymmetry with the rollback tests above: markFailed has no caller to
// return an error to, and unlike Pause/Resume/ForceStart it is never called
// from a stable status, so there is no earlier state that is both safe and
// worth restoring. Failed is itself the durable landing state that Resume
// and ForceStart both accept, so it is kept rather than reverted to
// whatever transient status preceded it.
func TestMarkFailedKeepsFailedStatusOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusDownloading)
	breakStore(t, m)

	m.markFailed("a", "boom", errors.New("cause"))

	if got := m.statusOf(t, "a"); got != StatusFailed {
		t.Fatalf("status = %s, want %s even though persist failed", got, StatusFailed)
	}
	if st := m.degradedStatus(); !st.Degraded || st.Message == "" {
		t.Fatalf("degraded status = %+v, want degraded with a message", st)
	}
}

// TestCompleteLockedKeepsCompletedStatusOnPersistFailure mirrors the
// markFailed case: by the time completeLocked runs, the files are already
// verified on disk, so the completion is real regardless of whether the
// journal write succeeds.
func TestCompleteLockedKeepsCompletedStatusOnPersistFailure(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusDownloading)
	breakStore(t, m)

	m.mu.Lock()
	m.completeLocked(d)
	m.mu.Unlock()

	if got := m.statusOf(t, "a"); got != StatusCompleted {
		t.Fatalf("status = %s, want %s even though persist failed", got, StatusCompleted)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded status = %+v, want degraded", st)
	}
}

func TestDegradedStatusClearsAfterSuccessfulPersist(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusQueued)
	m.addTestItem("b", StatusQueued)
	breakStore(t, m)
	if err := m.MoveDown("a"); err == nil {
		t.Fatal("setup: expected the move to fail")
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatal("setup: expected a degraded status")
	}

	m.mu.Lock()
	m.store.dir = t.TempDir()
	err := m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("persist after repair: %v", err)
	}
	if st := m.degradedStatus(); st.Degraded {
		t.Fatalf("degraded status = %+v, want cleared after a successful persist", st)
	}
}

func TestServiceShutdownReturnsPersistFailureButStillCleansUp(t *testing.T) {
	m := mustManagerAt(t, t.TempDir())
	if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	breakStore(t, m)

	err := m.ServiceShutdown()
	if err == nil {
		t.Fatal("ServiceShutdown must return the persist failure")
	}
	m.mu.Lock()
	cl, pc := m.client, m.pieceCompletion
	m.mu.Unlock()
	if cl != nil {
		t.Fatal("client was not released despite the persist failure")
	}
	if pc != nil {
		t.Fatal("piece completion store was not released despite the persist failure")
	}
}
