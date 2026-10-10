package download

import (
	"errors"
	"testing"
	"time"

	"typhon/internal/history"
)

func TestOnGoneFiresOnlyWhenADownloadWillNeverComplete(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		op     func(t *testing.T, m *Manager) error
		want   bool
	}{
		{"cancel", StatusQueued, func(_ *testing.T, m *Manager) error { return m.Cancel("a") }, true},
		{"remove", StatusPaused, func(_ *testing.T, m *Manager) error { return m.Remove("a") }, true},
		{"delete data", StatusCompleted, func(_ *testing.T, m *Manager) error { return m.DeleteData("a") }, true},
		{"stop and wait", StatusQueued, func(_ *testing.T, m *Manager) error { return m.StopAndWait("a") }, true},
		{"failure", StatusDownloading, func(_ *testing.T, m *Manager) error {
			m.markFailed("a", "boom", errors.New("cause"))
			return nil
		}, true},
		{"failure of a download that already failed", StatusFailed, func(_ *testing.T, m *Manager) error {
			m.markFailed("a", "boom", errors.New("cause"))
			return nil
		}, false},
		{"completion", StatusVerifying, func(_ *testing.T, m *Manager) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.completeLocked(m.findLocked("a"))
			return nil
		}, false},
		{"pause", StatusDownloading, func(_ *testing.T, m *Manager) error { return m.Pause("a") }, false},
		{"resume", StatusPaused, func(_ *testing.T, m *Manager) error { return m.Resume("a") }, false},
		{"a cancel whose record cannot be removed", StatusQueued, func(t *testing.T, m *Manager) error {
			breakStore(t, m)
			if err := m.Cancel("a"); err == nil {
				t.Fatal("the persist failure was swallowed")
			}
			return nil
		}, false},
		{"a cancel of an unknown download", StatusQueued, func(_ *testing.T, m *Manager) error {
			if err := m.Cancel("missing"); !errors.Is(err, errNotFound) {
				t.Fatalf("error = %v", err)
			}
			return nil
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 1)
			m.addTestItem("a", c.status)
			attachFake(m, "a")
			gone := make(chan string, 4)
			m.SetOnGone(func(id string) { gone <- id })

			if err := c.op(t, m); err != nil {
				t.Fatal(err)
			}
			m.wg.Wait()

			var got []string
			for len(gone) > 0 {
				got = append(got, <-gone)
			}
			if c.want && (len(got) != 1 || got[0] != "a") {
				t.Fatalf("onGone calls = %v, want exactly one for a", got)
			}
			if !c.want && len(got) != 0 {
				t.Fatalf("onGone calls = %v, want none", got)
			}
		})
	}
}

func TestOnGoneIsNotStartedOnceTheManagerIsClosing(t *testing.T) {
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusQueued)
	gone := make(chan string, 1)
	m.SetOnGone(func(id string) { gone <- id })
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()

	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()

	if len(gone) != 0 {
		t.Fatal("a callback was started after shutdown began")
	}
}

func TestStopAndWait(t *testing.T) {
	t.Run("a closing manager refuses", func(t *testing.T) {
		m := newTestManager(t, 1)
		m.addTestItem("a", StatusQueued)
		m.mu.Lock()
		m.closing = true
		m.mu.Unlock()
		if err := m.StopAndWait("a"); !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v", err)
		}
		if _, err := m.Get("a"); err != nil {
			t.Fatalf("download lost: %v", err)
		}
	})

	t.Run("an unknown download with nothing pending returns at once", func(t *testing.T) {
		m := newTestManager(t, 1)
		if err := m.StopAndWait("missing"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an unknown download waits for the teardown that is still running", func(t *testing.T) {
		m := newTestManager(t, 1)
		m.mu.Lock()
		done := m.trackTeardownLocked("a")
		m.mu.Unlock()
		returned := make(chan error, 1)
		go func() { returned <- m.StopAndWait("a") }()
		select {
		case err := <-returned:
			t.Fatalf("returned (%v) while the teardown was still running", err)
		case <-time.After(50 * time.Millisecond):
		}
		close(done)
		select {
		case err := <-returned:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("did not return after the teardown finished")
		}
	})

	t.Run("a running download is stopped and its job awaited", func(t *testing.T) {
		m := newTestManager(t, 1)
		m.addTestItem("a", StatusDownloading)
		eng := attachFake(m, "a")
		jobStopped := make(chan struct{})
		job := &jobState{done: make(chan struct{})}
		job.cancel = func() { close(jobStopped) }
		m.mu.Lock()
		m.jobs["a"] = job
		m.mu.Unlock()
		log := recordEmits(t)
		returned := make(chan error, 1)

		go func() { returned <- m.StopAndWait("a") }()
		<-jobStopped
		select {
		case err := <-returned:
			t.Fatalf("returned (%v) before the job ended", err)
		case <-time.After(50 * time.Millisecond):
		}
		if eng.wasDropped() {
			t.Fatal("the engine was dropped under a running job")
		}
		close(job.done)
		if err := <-returned; err != nil {
			t.Fatal(err)
		}

		if !eng.wasDropped() {
			t.Fatal("the engine was not dropped")
		}
		if _, err := m.Get("a"); !errors.Is(err, errNotFound) {
			t.Fatalf("the download is still tracked: %v", err)
		}
		if _, left := persistedStatuses(t, m)["a"]; left {
			t.Fatal("the download is still persisted")
		}
		if log.count(eventRemoved) != 1 {
			t.Fatalf("download:removed emitted %d times", log.count(eventRemoved))
		}
		m.mu.Lock()
		pending := len(m.teardowns)
		m.mu.Unlock()
		if pending != 0 {
			t.Fatalf("teardowns left = %d", pending)
		}
	})

	t.Run("a record that cannot be removed stays and keeps its engine", func(t *testing.T) {
		m := newTestManager(t, 1)
		m.addTestItem("a", StatusDownloading)
		eng := attachFake(m, "a")
		breakStore(t, m)

		if err := m.StopAndWait("a"); err == nil {
			t.Fatal("the persist failure was swallowed")
		}
		if _, err := m.Get("a"); err != nil {
			t.Fatalf("download lost: %v", err)
		}
		if eng.wasDropped() {
			t.Fatal("the engine was dropped although the record stays")
		}
	})
}

func TestCompletionIsWrittenToTheHistory(t *testing.T) {
	prepare := func(t *testing.T) (*Manager, *Download) {
		m := newTestManager(t, 1)
		d := m.addTestItem("a", StatusVerifying)
		setItem(m, "a", func(d *Download) {
			d.Name = "Great Game"
			d.Total = 4096
			d.Origin.GameID = "game-9"
		})
		return m, d
	}

	t.Run("the record names the game and the download", func(t *testing.T) {
		m, d := prepare(t)
		var got []history.Record
		m.SetHistoryRecorder(func(r history.Record) error {
			got = append(got, r)
			return nil
		})

		m.mu.Lock()
		m.completeLocked(d)
		m.mu.Unlock()

		if len(got) != 1 {
			t.Fatalf("records = %+v", got)
		}
		want := history.Record{Kind: history.KindDownloaded, GameID: "game-9", Title: "Great Game", Bytes: 4096, BytesKnown: true, RefID: "a"}
		if got[0] != want {
			t.Fatalf("record = %+v, want %+v", got[0], want)
		}
	})

	t.Run("a journal that fails does not fail the download", func(t *testing.T) {
		m, d := prepare(t)
		notified := make(chan Download, 1)
		m.SetOnCompleted(func(d Download) { notified <- d })
		m.SetHistoryRecorder(func(history.Record) error { return errors.New("journal full") })

		m.mu.Lock()
		m.completeLocked(d)
		m.mu.Unlock()
		m.wg.Wait()

		if got := m.statusOf(t, "a"); got != StatusCompleted {
			t.Fatalf("status = %s", got)
		}
		if got := persistedStatuses(t, m)["a"]; got != StatusCompleted {
			t.Fatalf("persisted status = %s", got)
		}
		select {
		case <-notified:
		default:
			t.Fatal("the installer was not told about the finished download")
		}
	})

	t.Run("no recorder is fine", func(t *testing.T) {
		m, d := prepare(t)
		m.SetHistoryRecorder(nil)
		m.mu.Lock()
		m.completeLocked(d)
		m.mu.Unlock()
		if got := m.statusOf(t, "a"); got != StatusCompleted {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("a cancelled or removed download leaves no record", func(t *testing.T) {
		m, _ := prepare(t)
		calls := 0
		m.SetHistoryRecorder(func(history.Record) error { calls++; return nil })
		if err := m.Cancel("a"); err != nil {
			t.Fatal(err)
		}
		m.wg.Wait()
		if calls != 0 {
			t.Fatalf("recorder called %d times", calls)
		}
	})
}
