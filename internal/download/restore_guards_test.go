package download

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/settings"
)

func TestSettleRestoredStaysInAUsableStatusWhenTheRecordsCannotBeSaved(t *testing.T) {
	cases := []struct {
		name string
		job  restoreJob
		want Status
	}{
		{"a paused download stays paused", restoreJob{id: "a", paused: true, trusted: true}, StatusPaused},
		{"a queued download is started by the queue", restoreJob{id: "a", trusted: true}, StatusDownloading},
		{"a forced download is started at once", restoreJob{id: "a", trusted: true, force: true}, StatusDownloading},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 1)
			m.addTestItem("a", StatusVerifying)
			blockDownloadsFile(t, m)
			eng := &fakeTorrent{size: 100}
			c.job.gen = m.gen

			m.settleRestored(t.Context(), c.job, eng, nil)

			if got := m.statusOf(t, "a"); got != c.want {
				t.Fatalf("status = %s, want %s: a status no operation accepts would strand the download", got, c.want)
			}
			if !hasEngine(m, "a") {
				t.Fatal("the restored engine was not kept")
			}
			if st := m.degradedStatus(); !st.Degraded {
				t.Fatalf("degraded = %+v, want the failed write surfaced", st)
			}
		})
	}
}

func TestSettleRestoredOfAFinishedDownloadFollowsTheSeedingSettingOfTheMoment(t *testing.T) {
	cases := []struct {
		name         string
		seedAfter    bool
		wantEngine   bool
		wantUploader bool
	}{
		{"seeding on keeps the engine and uploads", true, true, true},
		{"seeding switched off while the job ran drops the engine", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := settings.Defaults()
			cfg.SeedAfterDownload = c.seedAfter
			m, _ := newManagerWithSettings(t, cfg)
			m.addTestItem("a", StatusCompleted)
			eng := &fakeTorrent{size: 100}

			m.settleRestored(t.Context(), restoreJob{id: "a", complete: true, seeding: true, gen: m.gen}, eng, nil)
			m.wg.Wait()

			d := mustGet(t, m, "a")
			if d.Status != StatusCompleted || d.Seeding != c.seedAfter {
				t.Fatalf("download = %s seeding=%v", d.Status, d.Seeding)
			}
			if hasEngine(m, "a") != c.wantEngine || eng.wasDropped() == c.wantEngine {
				t.Fatalf("engine attached = %v, dropped = %v", hasEngine(m, "a"), eng.wasDropped())
			}
			if eng.isUploading() != c.wantUploader {
				t.Fatalf("uploading = %v", eng.isUploading())
			}
			if got := persistedRecords(t, m)[0].Seeding; got != c.seedAfter {
				t.Fatalf("persisted seeding = %v", got)
			}
		})
	}

	t.Run("a record that cannot be saved still ends completed", func(t *testing.T) {
		cfg := settings.Defaults()
		cfg.SeedAfterDownload = true
		m, _ := newManagerWithSettings(t, cfg)
		m.addTestItem("a", StatusCompleted)
		blockDownloadsFile(t, m)

		m.settleRestored(t.Context(), restoreJob{id: "a", complete: true, seeding: true, gen: m.gen}, &fakeTorrent{size: 100}, nil)

		if d := mustGet(t, m, "a"); d.Status != StatusCompleted || !d.Seeding {
			t.Fatalf("download = %s seeding=%v", d.Status, d.Seeding)
		}
		if st := m.degradedStatus(); !st.Degraded {
			t.Fatalf("degraded = %+v", st)
		}
	})
}

func TestSettleRestoredTakesTheFileListFromTheTorrentWhenTheDownloadHasNone(t *testing.T) {
	mi, _ := threeFileTorrent(t)
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusPaused)
	setItem(m, "a", func(d *Download) { d.Files, d.Total = nil, 0 })

	m.settleRestored(t.Context(), restoreJob{id: "a", paused: true, trusted: true, gen: m.gen}, &fakeTorrent{size: 100}, &info)

	d := mustGet(t, m, "a")
	if len(d.Files) != 3 || d.Total != 60000 || d.Status != StatusPaused {
		t.Fatalf("download = %+v", d)
	}
	for _, f := range d.Files {
		if !f.Selected {
			t.Fatalf("file %s is not selected", f.Path)
		}
	}
}

func TestRestoreOneStepsAsideWhenItsPremisesAreGone(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, m *Manager) (context.Context, restoreJob)
		check func(t *testing.T, m *Manager)
	}{
		{"the context of the pass is over", func(t *testing.T, m *Manager) (context.Context, restoreJob) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx, restoreJob{id: "a", infoHash: "aaaa", gen: m.gen}
		}, func(t *testing.T, m *Manager) {
			assertStatuses(t, m, map[string]Status{"a": StatusQueued})
		}},
		{"the client of the job has been replaced", func(t *testing.T, m *Manager) (context.Context, restoreJob) {
			return t.Context(), restoreJob{id: "a", infoHash: "aaaa", gen: m.gen + 5}
		}, func(t *testing.T, m *Manager) {
			assertStatuses(t, m, map[string]Status{"a": StatusQueued})
		}},
		{"another job already works on the download", func(t *testing.T, m *Manager) (context.Context, restoreJob) {
			m.mu.Lock()
			m.jobs["a"] = &jobState{cancel: func() {}, done: make(chan struct{})}
			m.mu.Unlock()
			return t.Context(), restoreJob{id: "a", infoHash: "aaaa", gen: m.gen}
		}, func(t *testing.T, m *Manager) {
			assertStatuses(t, m, map[string]Status{"a": StatusQueued})
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.jobs["a"] == nil {
				t.Error("the running job was removed by the second one")
			}
		}},
		{"a finished download whose torrent is held elsewhere stops seeding and is not failed", func(t *testing.T, m *Manager) (context.Context, restoreJob) {
			m.addTestItem("other", StatusDownloading)
			setItem(m, "other", func(d *Download) { d.InfoHash = "aaaa" })
			attachFake(m, "other")
			setItem(m, "a", func(d *Download) { d.Status, d.Seeding = StatusCompleted, true })
			return t.Context(), restoreJob{id: "a", infoHash: "aaaa", complete: true, seeding: true, gen: m.gen}
		}, func(t *testing.T, m *Manager) {
			d := mustGet(t, m, "a")
			if d.Status != StatusCompleted || d.Seeding || d.Error != "" {
				t.Fatalf("download = %+v", d)
			}
		}},
		{"a finished download whose torrent file is gone stops seeding and is not failed", func(t *testing.T, m *Manager) (context.Context, restoreJob) {
			setItem(m, "a", func(d *Download) {
				d.Status, d.Seeding = StatusCompleted, true
				d.Source = filepath.Join(t.TempDir(), "gone.torrent")
			})
			return t.Context(), restoreJob{id: "a", infoHash: "bbbb", source: mustGet(t, m, "a").Source, complete: true, seeding: true, gen: m.gen}
		}, func(t *testing.T, m *Manager) {
			d := mustGet(t, m, "a")
			if d.Status != StatusCompleted || d.Seeding || d.Error != "" {
				t.Fatalf("download = %+v", d)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := managerWithClient(t, 2)
			m.addTestItem("a", StatusQueued)
			setItem(m, "a", func(d *Download) { d.InfoHash = "aaaa" })
			cl := m.client
			ctx, job := c.setup(t, m)

			m.restoreOne(ctx, cl, job)
			m.wg.Wait()

			c.check(t, m)
			if hasEngine(m, "a") {
				t.Fatal("an engine was attached")
			}
			if got := len(cl.cl.Torrents()); got != 0 {
				t.Fatalf("torrents added to the client: %d", got)
			}
		})
	}
}

func TestVerifyCompletionBacksOffInsteadOfFailingTheDownload(t *testing.T) {
	prepare := func(t *testing.T) (*Manager, *fakeTorrent, string, []FileState) {
		m := newTestManager(t, 1)
		m.addTestItem("a", StatusVerifying)
		eng := attachFake(m, "a")
		dir := t.TempDir()
		eng.mu.Lock()
		eng.paths = []string{filepath.Join(dir, "missing.bin")}
		eng.mu.Unlock()
		return m, eng, dir, []FileState{{Path: "missing.bin", Size: 100, Selected: true}}
	}

	t.Run("the context ends during the check", func(t *testing.T) {
		m, eng, dir, files := prepare(t)
		log := recordEmits(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		m.verifyCompletion(ctx, "a", eng, dir, files)

		if got := m.statusOf(t, "a"); got != StatusVerifying {
			t.Fatalf("status = %s: a check that was interrupted says nothing about the files", got)
		}
		if log.count(eventFailed) != 0 {
			t.Fatal("download:failed was emitted for an interrupted check")
		}
		m.mu.Lock()
		jobs := len(m.jobs)
		m.mu.Unlock()
		if jobs != 0 {
			t.Fatalf("jobs left = %d", jobs)
		}
	})

	t.Run("a job already works on the download", func(t *testing.T) {
		m, eng, dir, files := prepare(t)
		job := &jobState{cancel: func() {}, done: make(chan struct{})}
		m.mu.Lock()
		m.jobs["a"] = job
		m.mu.Unlock()

		m.verifyCompletion(t.Context(), "a", eng, dir, files)

		if got := m.statusOf(t, "a"); got != StatusVerifying {
			t.Fatalf("status = %s", got)
		}
		m.mu.Lock()
		same := m.jobs["a"] == job
		m.mu.Unlock()
		if !same {
			t.Fatal("the second check removed the job of the first")
		}
	})

	t.Run("a missing file with a live context does fail the download", func(t *testing.T) {
		m, eng, dir, files := prepare(t)

		m.verifyCompletion(t.Context(), "a", eng, dir, files)

		if got := m.statusOf(t, "a"); got != StatusFailed {
			t.Fatalf("status = %s", got)
		}
	})
}

func TestSeedingTurnedOnForADownloadThatCannotBeRestoredIsNotClaimed(t *testing.T) {
	cfg := settings.Defaults()
	m, svc := newManagerWithSettings(t, cfg)
	m.client = offlineClient(t)
	m.addTestItem("a", StatusCompleted)
	setItem(m, "a", func(d *Download) { d.Source = filepath.Join(t.TempDir(), "gone.torrent") })
	next := cfg
	next.SeedAfterDownload = true
	if err := svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	m.applySettings(next)
	m.wg.Wait()

	if d := mustGet(t, m, "a"); d.Seeding || d.Status != StatusCompleted {
		t.Fatalf("download = %s seeding=%v: nothing is seeding it", d.Status, d.Seeding)
	}
	m.mu.Lock()
	jobs := len(m.jobs)
	m.mu.Unlock()
	if jobs != 0 || hasEngine(m, "a") {
		t.Fatalf("a restore was started for a download with no torrent to restore: jobs = %d", jobs)
	}
}

func TestPauseAndResumeClearTheStall(t *testing.T) {
	withStallAfter(t, 0)
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusQueued)
	eng := attachFake(m, "a")
	base := time.Now()
	m.mu.Lock()
	m.schedule()
	m.mu.Unlock()
	m.sample(m.ctx, base)
	m.sample(m.ctx, base.Add(time.Second))
	if !mustGet(t, m, "a").Stalled {
		t.Fatal("setup: the download should be stalled")
	}

	if err := m.Pause("a"); err != nil {
		t.Fatal(err)
	}
	if d := mustGet(t, m, "a"); d.Stalled || d.StalledSince != nil {
		t.Fatalf("a paused download is still marked stalled: %+v", d)
	}
	if err := m.Resume("a"); err != nil {
		t.Fatal(err)
	}
	if d := mustGet(t, m, "a"); d.Stalled || d.StalledSince != nil || !eng.isDownloading() {
		t.Fatalf("a resumed download starts with a clean stall clock: %+v", d)
	}
}
