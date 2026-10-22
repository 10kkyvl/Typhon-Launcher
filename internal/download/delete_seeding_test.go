package download

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

type probingEngine struct {
	*fakeTorrent
	probe string

	probeMu     sync.Mutex
	presentOnce bool
}

func (p *probingEngine) drop() {
	_, err := os.Stat(p.probe)
	p.probeMu.Lock()
	p.presentOnce = err == nil
	p.probeMu.Unlock()
	p.fakeTorrent.drop()
}

func (p *probingEngine) filesWerePresentAtDrop() bool {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	return p.presentOnce
}

func seedingWithFolder(t *testing.T) (*Manager, string, *probingEngine) {
	t.Helper()
	m, root := completedWithFolder(t)
	eng := &probingEngine{
		fakeTorrent: &fakeTorrent{size: 100, uploading: true},
		probe:       filepath.Join(root, "game.bin"),
	}
	m.mu.Lock()
	m.findLocked("a").Seeding = true
	m.engines["a"] = eng
	m.mu.Unlock()
	return m, root, eng
}

func TestDeleteDataStopsSeedingAndRemovesFiles(t *testing.T) {
	m, root, eng := seedingWithFolder(t)
	log := recordEmits(t)

	if err := m.DeleteData("a"); err != nil {
		t.Fatalf("DeleteData: %v", err)
	}

	if !eng.wasDropped() {
		t.Fatal("the engine was not dropped")
	}
	if eng.isUploading() {
		t.Fatal("the engine still uploads")
	}
	if !eng.filesWerePresentAtDrop() {
		t.Fatal("files were removed before the engine let go of them")
	}
	m.mu.Lock()
	_, attached := m.engines["a"]
	m.mu.Unlock()
	if attached {
		t.Fatal("the engine is still attached")
	}
	if _, err := m.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if _, left := persistedStatuses(t, m)["a"]; left {
		t.Fatal("the record is still persisted")
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder survived: %v", err)
	}
	if got := log.count(eventRemoved); got != 1 {
		t.Fatalf("download:removed emitted %d times, want 1", got)
	}
	if pending := m.PendingCleanups(); len(pending) != 0 {
		t.Fatalf("cleanups left = %+v", pending)
	}
}

func TestDeleteDataKeepsSeedingWhenTheRecordWillNotGo(t *testing.T) {
	m, root, eng := seedingWithFolder(t)
	breakStore(t, m)

	err := m.DeleteData("a")
	if !errors.Is(err, errSeeding) {
		t.Fatalf("DeleteData = %v, want errSeeding", err)
	}
	if err.Error() == errSeeding.Error() {
		t.Fatalf("the cause of the failure is lost: %v", err)
	}
	if eng.wasDropped() {
		t.Fatal("the engine was dropped although the record stayed")
	}
	if !eng.isUploading() {
		t.Fatal("seeding was silenced although the record stayed")
	}
	d, getErr := m.Get("a")
	if getErr != nil || !d.Seeding || d.Deleting() {
		t.Fatalf("download after the failure = %+v, %v; want it seeding and unclaimed", d, getErr)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("files removed despite the failure: %v", statErr)
	}
}

func TestDeleteDataRetriesFilesTheDroppedEngineStillHolds(t *testing.T) {
	busy := errors.New("sharing violation")
	m, root, _ := seedingWithFolder(t)

	oldDelays, oldBusy, oldRemove := busyRetryDelays, fileBusy, removeAll
	busyRetryDelays = []time.Duration{time.Nanosecond, time.Nanosecond, time.Nanosecond}
	fileBusy = func(err error) bool { return errors.Is(err, busy) }
	var mu sync.Mutex
	failures := 2
	removeAll = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		if path == root && failures > 0 {
			failures--
			return busy
		}
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { busyRetryDelays, fileBusy, removeAll = oldDelays, oldBusy, oldRemove })

	if err := m.DeleteData("a"); err != nil {
		t.Fatalf("DeleteData: %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder survived: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if failures != 0 {
		t.Fatalf("busy failures left = %d, the removal was not retried", failures)
	}
}

func TestDeleteDataReportsFilesThatStayBusy(t *testing.T) {
	busy := errors.New("sharing violation")
	m, root, _ := seedingWithFolder(t)

	oldDelays, oldBusy, oldRemove := busyRetryDelays, fileBusy, removeAll
	busyRetryDelays = []time.Duration{time.Nanosecond, time.Nanosecond}
	fileBusy = func(err error) bool { return errors.Is(err, busy) }
	removeAll = func(string) error { return busy }
	t.Cleanup(func() { busyRetryDelays, fileBusy, removeAll = oldDelays, oldBusy, oldRemove })

	err := m.DeleteData("a")
	if !errors.Is(err, errDataLeft) || !errors.Is(err, busy) {
		t.Fatalf("DeleteData = %v, want errDataLeft wrapping the cause", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("folder must still be there: %v", statErr)
	}
	if pending := m.PendingCleanups(); len(pending) == 0 {
		t.Fatal("leftover files are not tracked for the next start")
	}
}

func TestDeleteDataStopsSeedingUnderConcurrentSync(t *testing.T) {
	m, root, eng := seedingWithFolder(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				m.mu.Lock()
				m.syncPlayLocked()
				m.mu.Unlock()
				m.sample(m.ctx, time.Now())
			}
		}()
	}

	err := m.DeleteData("a")
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("DeleteData: %v", err)
	}

	m.mu.Lock()
	m.syncPlayLocked()
	_, attached := m.engines["a"]
	m.mu.Unlock()
	if attached {
		t.Fatal("the engine is attached to a deleted download")
	}
	if eng.isUploading() {
		t.Fatal("a deleted download is still uploading")
	}
	if !eng.wasDropped() {
		t.Fatal("the engine was not dropped")
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("folder survived: %v", statErr)
	}
}

func TestDeleteDataReportsAnUnfinishedRemovalWhenTheManagerCloses(t *testing.T) {
	m, root := completedWithFolder(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	oldRemove := removeAll
	removeAll = func(path string) error {
		once.Do(func() { close(entered) })
		<-release
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { removeAll = oldRemove })
	t.Cleanup(func() {
		close(release)
		m.wg.Wait()
	})

	result := make(chan error, 1)
	go func() { result <- m.DeleteData("a") }()
	<-entered
	m.cancel()

	err := <-result
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("DeleteData = %v, want errUnavailable: the removal is not finished", err)
	}
	if got := uierr.Code(err); got != "download.unavailable" {
		t.Fatalf("code = %q", got)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("folder must still be there: %v", statErr)
	}
}

func TestRetryBusyReturnsTheLastErrorWhenDoneIsClosed(t *testing.T) {
	busy := errors.New("sharing violation")
	oldDelays, oldBusy := busyRetryDelays, fileBusy
	busyRetryDelays = []time.Duration{time.Hour}
	fileBusy = func(err error) bool { return errors.Is(err, busy) }
	t.Cleanup(func() { busyRetryDelays, fileBusy = oldDelays, oldBusy })

	done := make(chan struct{})
	close(done)
	calls := 0
	err := retryBusy(done, func() error {
		calls++
		return busy
	})
	if !errors.Is(err, busy) {
		t.Fatalf("retryBusy = %v, want the busy error", err)
	}
	if calls != 1 {
		t.Fatalf("remove called %d times, want 1: nothing may be retried after done", calls)
	}
}

func TestStopSeedingDropsTheEngineAndKeepsTheRecord(t *testing.T) {
	m, root, eng := seedingWithFolder(t)
	log := recordEmits(t)

	if err := m.StopSeeding("a"); err != nil {
		t.Fatalf("StopSeeding: %v", err)
	}

	if !eng.wasDropped() {
		t.Fatal("the engine was not dropped")
	}
	if eng.isUploading() {
		t.Fatal("the engine still uploads")
	}
	if !eng.filesWerePresentAtDrop() {
		t.Fatal("files vanished while the engine was being dropped")
	}
	d, err := m.Get("a")
	if err != nil {
		t.Fatalf("Get after StopSeeding: %v", err)
	}
	if d.Seeding || d.Status != StatusCompleted {
		t.Fatalf("download = seeding %v, status %s; want completed and not seeding", d.Seeding, d.Status)
	}
	m.mu.Lock()
	_, attached := m.engines["a"]
	_, tearing := m.teardowns["a"]
	m.mu.Unlock()
	if attached {
		t.Fatal("the engine is still attached")
	}
	if tearing {
		t.Fatal("the teardown is still registered")
	}
	found := false
	for _, r := range persistedRecords(t, m) {
		if r.ID == "a" {
			found = true
			if r.Seeding {
				t.Fatal("the persisted record still says seeding")
			}
		}
	}
	if !found {
		t.Fatal("the record is not persisted")
	}
	if _, statErr := os.Stat(filepath.Join(root, "game.bin")); statErr != nil {
		t.Fatalf("files must stay: %v", statErr)
	}
	if got := log.count(eventUpdated); got != 1 {
		t.Fatalf("download:updated emitted %d times, want 1", got)
	}
	if got := log.count(eventRemoved); got != 0 {
		t.Fatalf("download:removed emitted %d times, want 0", got)
	}
}

func TestStopSeedingRollsBackWhenTheRecordWillNotSave(t *testing.T) {
	m, _, eng := seedingWithFolder(t)
	breakStore(t, m)

	if err := m.StopSeeding("a"); err == nil {
		t.Fatal("StopSeeding must return the persist failure")
	}
	if eng.wasDropped() {
		t.Fatal("the engine was dropped although the state was not saved")
	}
	if !eng.isUploading() {
		t.Fatal("seeding was silenced although the state was not saved")
	}
	d, err := m.Get("a")
	if err != nil || !d.Seeding {
		t.Fatalf("download = %+v, %v; want it still seeding", d, err)
	}
	m.mu.Lock()
	_, attached := m.engines["a"]
	m.mu.Unlock()
	if !attached {
		t.Fatal("the engine was detached although the state was not saved")
	}
}

func TestStopSeedingIsIdempotent(t *testing.T) {
	m, _, eng := seedingWithFolder(t)
	if err := m.StopSeeding("a"); err != nil {
		t.Fatalf("first StopSeeding: %v", err)
	}
	log := recordEmits(t)
	if err := m.StopSeeding("a"); err != nil {
		t.Fatalf("second StopSeeding: %v", err)
	}
	if got := log.count(eventUpdated); got != 0 {
		t.Fatalf("a repeated stop emitted %d updates, want 0", got)
	}
	if !eng.wasDropped() {
		t.Fatal("the engine was not dropped")
	}
}

func TestStopSeedingRefusals(t *testing.T) {
	t.Run("unknown download", func(t *testing.T) {
		m := newTestManager(t, 2)
		if err := m.StopSeeding("nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("StopSeeding = %v, want ErrNotFound", err)
		}
	})
	t.Run("manager closing", func(t *testing.T) {
		m, _, eng := seedingWithFolder(t)
		m.mu.Lock()
		m.closing = true
		m.mu.Unlock()
		if err := m.StopSeeding("a"); !errors.Is(err, errUnavailable) {
			t.Fatalf("StopSeeding = %v, want errUnavailable", err)
		}
		if eng.wasDropped() {
			t.Fatal("the engine was dropped while closing")
		}
	})
	t.Run("unfinished download keeps its engine", func(t *testing.T) {
		m := newTestManager(t, 2)
		eng := m.addTestDownload("a")
		if err := m.StopSeeding("a"); err != nil {
			t.Fatalf("StopSeeding = %v, want nil: nothing seeds", err)
		}
		if eng.wasDropped() || !eng.downloading {
			t.Fatal("a running download was torn down")
		}
	})
}

func TestStopSeedingWithoutAnEngineClearsTheWish(t *testing.T) {
	m, root := completedWithFolder(t)
	setItem(m, "a", func(d *Download) { d.Seeding = true })

	if err := m.StopSeeding("a"); err != nil {
		t.Fatalf("StopSeeding: %v", err)
	}
	d, err := m.Get("a")
	if err != nil || d.Seeding {
		t.Fatalf("download = %+v, %v; want it kept and not seeding", d, err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("files must stay: %v", statErr)
	}
}

func TestStopSeedingWaitsForARestoreJobInFlight(t *testing.T) {
	cases := []struct {
		name  string
		lands bool
	}{
		{name: "job ends without an engine"},
		{name: "job lands with an engine after the cancel", lands: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, eng := seedingWithFolder(t)
			late := &probingEngine{fakeTorrent: &fakeTorrent{size: 100, uploading: true}, probe: filepath.Join(t.TempDir(), "none")}
			if tc.lands {
				m.mu.Lock()
				delete(m.engines, "a")
				m.findLocked("a").Seeding = false
				m.mu.Unlock()
			}

			cancelled := make(chan struct{})
			job := &jobState{done: make(chan struct{})}
			job.cancel = func() { close(cancelled) }
			m.mu.Lock()
			m.jobs["a"] = job
			m.mu.Unlock()
			go func() {
				<-cancelled
				m.mu.Lock()
				if tc.lands {
					m.engines["a"] = late
					m.findLocked("a").Seeding = true
				}
				delete(m.jobs, "a")
				m.mu.Unlock()
				close(job.done)
			}()

			if err := m.StopSeeding("a"); err != nil {
				t.Fatalf("StopSeeding: %v", err)
			}
			select {
			case <-job.done:
			default:
				t.Fatal("StopSeeding returned before the job was done")
			}
			d, err := m.Get("a")
			if err != nil || d.Seeding {
				t.Fatalf("download = %+v, %v; want it kept and not seeding", d, err)
			}
			m.mu.Lock()
			_, attached := m.engines["a"]
			m.mu.Unlock()
			if attached {
				t.Fatal("an engine is attached to a download that stopped seeding")
			}
			stopped := eng.fakeTorrent
			if tc.lands {
				stopped = late.fakeTorrent
			}
			if !stopped.wasDropped() || stopped.isUploading() {
				t.Fatal("the engine that outlived the job was not stopped")
			}
		})
	}
}

func TestStopSeedingUnderConcurrentSync(t *testing.T) {
	m, root, eng := seedingWithFolder(t)
	next := settings.Defaults()
	next.SeedAfterDownload = true

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				m.applySettings(next)
				m.mu.Lock()
				m.syncPlayLocked()
				m.mu.Unlock()
				m.sample(m.ctx, time.Now())
			}
		}()
	}

	err := m.StopSeeding("a")
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("StopSeeding: %v", err)
	}

	m.mu.Lock()
	m.syncPlayLocked()
	_, attached := m.engines["a"]
	m.mu.Unlock()
	if attached {
		t.Fatal("the engine is attached to a download that stopped seeding")
	}
	if eng.isUploading() || !eng.wasDropped() {
		t.Fatal("the engine was not stopped and dropped")
	}
	d, getErr := m.Get("a")
	if getErr != nil || d.Seeding {
		t.Fatalf("download = %+v, %v; want it kept and not seeding", d, getErr)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("files must stay: %v", statErr)
	}
}
