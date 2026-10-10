package download

import (
	"errors"
	"testing"
	"time"
)

func downloadWithoutEngine(t *testing.T, m *Manager, id string, status Status) *Download {
	t.Helper()
	d, _ := realDownload(t, m, id, status, false)
	setItem(m, id, func(d *Download) {
		d.Destination = t.TempDir()
		d.InPlace = false
	})
	return d
}

func TestResumeAndForceStartWithoutAnEngineRestoreItThroughACheck(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		start  func(m *Manager) error
		slots  int
		want   Status
	}{
		{"resume of a paused download with a free slot", StatusPaused, func(m *Manager) error { return m.Resume("a") }, 2, StatusDownloading},
		{"resume of a failed download with a free slot", StatusFailed, func(m *Manager) error { return m.Resume("a") }, 2, StatusDownloading},
		{"resume waits when every slot is busy", StatusPaused, func(m *Manager) error { return m.Resume("a") }, 1, StatusQueued},
		{"a forced start ignores the busy slots", StatusPaused, func(m *Manager) error { return m.ForceStart("a") }, 1, StatusDownloading},
		{"a forced start of a failed download", StatusFailed, func(m *Manager) error { return m.ForceStart("a") }, 1, StatusDownloading},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := managerWithClient(t, c.slots)
			log := recordEmits(t)
			m.addTestDownload("busy")
			downloadWithoutEngine(t, m, "a", c.status)
			setItem(m, "a", func(d *Download) { d.Error = map[bool]string{true: "disk full", false: ""}[c.status == StatusFailed] })

			if err := c.start(m); err != nil {
				t.Fatalf("start: %v", err)
			}
			if got := m.statusOf(t, "a"); got != StatusVerifying {
				t.Fatalf("status right after the call = %s, want it to be checked first", got)
			}
			if got := persistedStatuses(t, m)["a"]; got != StatusVerifying {
				t.Fatalf("persisted status = %s", got)
			}

			waitUntil(t, "the restore to settle", func() bool { return m.statusOf(t, "a") != StatusVerifying && hasEngine(m, "a") })
			m.wg.Wait()

			if got := m.statusOf(t, "a"); got != c.want {
				t.Fatalf("status = %s, want %s", got, c.want)
			}
			if got := mustGet(t, m, "a").Error; got != "" {
				t.Fatalf("error = %q, want it cleared", got)
			}
			if !containsStatus(log.statusesOf("a"), StatusVerifying) {
				t.Fatal("the download was not announced as verifying")
			}
			if got := len(m.client.cl.Torrents()); got != 1 {
				t.Fatalf("torrents in the client = %d, want only the restored download", got)
			}
		})
	}
}

func TestResumeWithoutAnEngineRollsBackOnPersistFailure(t *testing.T) {
	m := managerWithClient(t, 2)
	downloadWithoutEngine(t, m, "a", StatusPaused)
	blockDownloadsFile(t, m)

	err := m.Resume("a")

	if err == nil {
		t.Fatal("the persist failure was swallowed")
	}
	if got := m.statusOf(t, "a"); got != StatusPaused {
		t.Fatalf("status = %s, want the pause to stay", got)
	}
	m.wg.Wait()
	m.mu.Lock()
	jobs := len(m.jobs)
	m.mu.Unlock()
	if jobs != 0 || hasEngine(m, "a") || len(m.client.cl.Torrents()) != 0 {
		t.Fatalf("a restore was started for a request that was refused: jobs = %d", jobs)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded = %+v", st)
	}
}

func TestResumeWhileARestoreIsRunningIsRefused(t *testing.T) {
	m := managerWithClient(t, 2)
	downloadWithoutEngine(t, m, "a", StatusFailed)
	m.mu.Lock()
	m.jobs["a"] = &jobState{cancel: func() {}, done: make(chan struct{})}
	m.mu.Unlock()

	for name, call := range map[string]func() error{
		"resume":      func() error { return m.Resume("a") },
		"force start": func() error { return m.ForceStart("a") },
	} {
		if err := call(); !errors.Is(err, errUnavailable) {
			t.Fatalf("%s during a restore = %v, want errUnavailable", name, err)
		}
	}
	if got := m.statusOf(t, "a"); got != StatusFailed {
		t.Fatalf("status = %s", got)
	}
}

func TestRestoreOfADownloadWhoseTorrentIsHeldElsewhereFailsItWithThatReason(t *testing.T) {
	m := managerWithClient(t, 2)
	holder, _ := realDownload(t, m, "holder", StatusDownloading, false)
	attachFake(m, "holder")
	downloadWithoutEngine(t, m, "a", StatusPaused)
	setItem(m, "a", func(d *Download) { d.InfoHash = holder.InfoHash })

	if err := m.Resume("a"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the restore to give up", func() bool { return m.statusOf(t, "a") == StatusFailed })
	m.wg.Wait()

	if got := mustGet(t, m, "a").Error; got != errHashBusy.Error() {
		t.Fatalf("error = %q, want %q", got, errHashBusy.Error())
	}
	if got := m.statusOf(t, "holder"); got != StatusDownloading {
		t.Fatalf("the download that owns the torrent was disturbed: %s", got)
	}
	if hasEngine(m, "a") {
		t.Fatal("two downloads share one torrent")
	}
	if _, reserved := m.bookkeeping(); reserved != 0 {
		t.Fatalf("reserved = %d", reserved)
	}
}

func TestRestoreFromAMagnetWithoutAnyoneAnsweringEndsFailedAndCanBeCancelled(t *testing.T) {
	const hash = "a748597437835a2fd0d2e06f8edd86fee316a84d"

	t.Run("the wait for metadata runs out", func(t *testing.T) {
		old := metadataTimeout
		metadataTimeout = 20 * time.Millisecond
		t.Cleanup(func() { metadataTimeout = old })
		m := managerWithClient(t, 2)
		m.addTestItem("a", StatusPaused)
		setItem(m, "a", func(d *Download) {
			d.InfoHash = hash
			d.Source = "magnet:?xt=urn:btih:" + hash
			d.Destination = t.TempDir()
		})

		if err := m.Resume("a"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "the restore to give up", func() bool { return m.statusOf(t, "a") == StatusFailed })
		m.wg.Wait()

		if got := mustGet(t, m, "a").Error; got != errNoRestore.Error() {
			t.Fatalf("error = %q, want %q", got, errNoRestore.Error())
		}
		if got := len(m.client.cl.Torrents()); got != 0 {
			t.Fatalf("torrents left in the client: %d", got)
		}
		if hasEngine(m, "a") {
			t.Fatal("an engine was attached to a download without metadata")
		}
	})

	t.Run("cancelling during the wait", func(t *testing.T) {
		m := managerWithClient(t, 2)
		m.addTestItem("a", StatusPaused)
		setItem(m, "a", func(d *Download) {
			d.InfoHash = hash
			d.Source = "magnet:?xt=urn:btih:" + hash
			d.Destination = t.TempDir()
		})
		if err := m.Resume("a"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "the restore to wait for metadata", func() bool { return m.statusOf(t, "a") == StatusMetadata })

		if err := m.Cancel("a"); err != nil {
			t.Fatal(err)
		}
		m.wg.Wait()

		if _, err := m.Get("a"); !errors.Is(err, errNotFound) {
			t.Fatalf("the download is still tracked: %v", err)
		}
		if got := len(m.client.cl.Torrents()); got != 0 {
			t.Fatalf("torrents left in the client: %d", got)
		}
		if _, reserved := m.bookkeeping(); reserved != 0 {
			t.Fatalf("reserved = %d", reserved)
		}
	})
}
