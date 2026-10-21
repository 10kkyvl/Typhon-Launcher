package download

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
