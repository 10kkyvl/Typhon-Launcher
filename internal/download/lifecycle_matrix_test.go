package download

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/settings"
)

type outcome struct {
	status Status
	err    error
	gone   bool
}

func becomes(s Status) outcome { return outcome{status: s} }
func refused(s Status) outcome { return outcome{status: s, err: errUnavailable} }

var removed = outcome{gone: true}

var allStatuses = []Status{
	StatusQueued, StatusMetadata, StatusDownloading, StatusPaused,
	StatusVerifying, StatusCompleted, StatusFailed,
}

func TestStatusTransitionMatrix(t *testing.T) {
	ops := []struct {
		name string
		call func(m *Manager) error
		want map[Status]outcome
	}{
		{"Pause", func(m *Manager) error { return m.Pause("a") }, map[Status]outcome{
			StatusQueued:      becomes(StatusPaused),
			StatusMetadata:    refused(StatusMetadata),
			StatusDownloading: becomes(StatusPaused),
			StatusPaused:      refused(StatusPaused),
			StatusVerifying:   refused(StatusVerifying),
			StatusCompleted:   refused(StatusCompleted),
			StatusFailed:      refused(StatusFailed),
		}},
		{"Resume", func(m *Manager) error { return m.Resume("a") }, map[Status]outcome{
			StatusQueued:      refused(StatusQueued),
			StatusMetadata:    refused(StatusMetadata),
			StatusDownloading: refused(StatusDownloading),
			StatusPaused:      becomes(StatusDownloading),
			StatusVerifying:   refused(StatusVerifying),
			StatusCompleted:   refused(StatusCompleted),
			StatusFailed:      becomes(StatusDownloading),
		}},
		{"ForceStart", func(m *Manager) error { return m.ForceStart("a") }, map[Status]outcome{
			StatusQueued:      becomes(StatusDownloading),
			StatusMetadata:    refused(StatusMetadata),
			StatusDownloading: refused(StatusDownloading),
			StatusPaused:      becomes(StatusDownloading),
			StatusVerifying:   refused(StatusVerifying),
			StatusCompleted:   refused(StatusCompleted),
			StatusFailed:      becomes(StatusDownloading),
		}},
		{"MoveUp", func(m *Manager) error { return m.MoveUp("a") }, map[Status]outcome{
			StatusQueued:      becomes(StatusQueued),
			StatusMetadata:    refused(StatusMetadata),
			StatusDownloading: refused(StatusDownloading),
			StatusPaused:      refused(StatusPaused),
			StatusVerifying:   refused(StatusVerifying),
			StatusCompleted:   refused(StatusCompleted),
			StatusFailed:      refused(StatusFailed),
		}},
		{"Cancel", func(m *Manager) error { return m.Cancel("a") }, allRemoved()},
		{"Remove", func(m *Manager) error { return m.Remove("a") }, allRemoved()},
		{"DeleteData", func(m *Manager) error { return m.DeleteData("a") }, map[Status]outcome{
			StatusQueued:      refused(StatusQueued),
			StatusMetadata:    refused(StatusMetadata),
			StatusDownloading: refused(StatusDownloading),
			StatusPaused:      refused(StatusPaused),
			StatusVerifying:   refused(StatusVerifying),
			StatusCompleted:   removed,
			StatusFailed:      refused(StatusFailed),
		}},
	}
	for _, op := range ops {
		for _, from := range allStatuses {
			want, ok := op.want[from]
			if !ok {
				t.Fatalf("%s has no expectation for %s", op.name, from)
			}
			t.Run(op.name+"/from_"+string(from), func(t *testing.T) {
				m := newTestManager(t, 1)
				m.addTestItem("a", from)
				eng := attachFake(m, "a")

				err := op.call(m)
				m.wg.Wait()

				if want.err == nil && err != nil {
					t.Fatalf("%s from %s: %v", op.name, from, err)
				}
				if want.err != nil && !errors.Is(err, want.err) {
					t.Fatalf("%s from %s: error = %v, want %v", op.name, from, err, want.err)
				}
				persisted := persistedStatuses(t, m)
				if want.gone {
					if _, err := m.Get("a"); !errors.Is(err, errNotFound) {
						t.Fatalf("download still tracked: %v", err)
					}
					if _, left := persisted["a"]; left {
						t.Fatalf("download still persisted as %s", persisted["a"])
					}
					return
				}
				if got := m.statusOf(t, "a"); got != want.status {
					t.Fatalf("status = %s, want %s", got, want.status)
				}
				if got, was := reloadedStatus(persisted["a"]), reloadedStatus(want.status); got != was {
					t.Fatalf("a restart would see %s, want %s", got, was)
				}
				if want.status == from {
					return
				}
				switch want.status {
				case StatusDownloading:
					if !eng.isDownloading() {
						t.Fatal("the operation made the download run but left its engine gated off")
					}
				case StatusPaused:
					if eng.isDownloading() || eng.isUploading() {
						t.Fatal("the operation paused the download but its engine still moves data")
					}
				}
			})
		}
	}
}

func allRemoved() map[Status]outcome {
	out := map[Status]outcome{}
	for _, s := range allStatuses {
		out[s] = removed
	}
	return out
}

func reloadedStatus(s Status) Status {
	if occupiesSlot(s) {
		return StatusQueued
	}
	return s
}

func (f *fakeTorrent) isDownloading() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.downloading
}

type world struct {
	t       *testing.T
	m       *Manager
	engines map[string]*fakeTorrent
}

func newWorld(t *testing.T, max int) *world {
	t.Helper()
	return &world{t: t, m: newTestManager(t, max), engines: map[string]*fakeTorrent{}}
}

func (w *world) add(id string) *fakeTorrent {
	w.t.Helper()
	eng := w.m.addTestDownload(id)
	w.engines[id] = eng
	return eng
}

func (w *world) expect(want map[string]Status) {
	w.t.Helper()
	persisted := persistedStatuses(w.t, w.m)
	for id, status := range want {
		if got := w.m.statusOf(w.t, id); got != status {
			w.t.Fatalf("%s status = %s, want %s", id, got, status)
		}
		if got, was := reloadedStatus(persisted[id]), reloadedStatus(status); got != was {
			w.t.Fatalf("%s is persisted so that a restart sees %s, want %s", id, got, was)
		}
	}
}

func (w *world) expectGone(ids ...string) {
	w.t.Helper()
	persisted := persistedStatuses(w.t, w.m)
	for _, id := range ids {
		if _, err := w.m.Get(id); !errors.Is(err, errNotFound) {
			w.t.Fatalf("%s is still tracked: %v", id, err)
		}
		if _, left := persisted[id]; left {
			w.t.Fatalf("%s is still persisted", id)
		}
	}
}

func (w *world) must(err error) {
	w.t.Helper()
	if err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) refuses(err, want error) {
	w.t.Helper()
	if !errors.Is(err, want) {
		w.t.Fatalf("error = %v, want %v", err, want)
	}
}

func (w *world) finish(id, path string) {
	w.t.Helper()
	eng := w.engines[id]
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{path}
	eng.mu.Unlock()
	w.m.sample(w.m.ctx, time.Now())
}

func (w *world) settle(id string, want Status) {
	w.t.Helper()
	waitUntil(w.t, id+" to reach "+string(want), func() bool { return w.m.statusOf(w.t, id) == want })
}

func TestStatusSequences(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"a pause hands its slot to the queue and a resume waits for a free one", func(t *testing.T) {
			w := newWorld(t, 1)
			a, b := w.add("a"), w.add("b")
			w.expect(map[string]Status{"a": StatusDownloading, "b": StatusQueued})

			w.must(w.m.Pause("a"))
			w.expect(map[string]Status{"a": StatusPaused, "b": StatusDownloading})
			if a.isDownloading() || a.isUploading() || !b.isDownloading() {
				t.Fatal("pausing a must gate its engine and let b run")
			}

			w.must(w.m.Resume("a"))
			w.expect(map[string]Status{"a": StatusQueued, "b": StatusDownloading})

			w.must(w.m.ForceStart("a"))
			w.expect(map[string]Status{"a": StatusDownloading, "b": StatusDownloading})

			w.must(w.m.Pause("b"))
			w.must(w.m.Cancel("a"))
			w.m.wg.Wait()
			w.expectGone("a")
			w.expect(map[string]Status{"b": StatusPaused})
			if !a.wasDropped() {
				t.Fatal("the engine of a cancelled download was not dropped")
			}
		}},
		{"a finished download is verified, completed and the next one starts", func(t *testing.T) {
			w := newWorld(t, 1)
			a := w.add("a")
			w.add("b")
			w.expect(map[string]Status{"a": StatusDownloading, "b": StatusQueued})

			w.finish("a", fakeFilePath(t, 100))
			w.settle("a", StatusCompleted)
			w.m.wg.Wait()
			w.expect(map[string]Status{"a": StatusCompleted, "b": StatusDownloading})

			done := mustGet(t, w.m, "a")
			if done.Seeding || done.CompletedAt == nil || done.Progress != 1 {
				t.Fatalf("completed download = %+v", done)
			}
			rec := persistedRecords(t, w.m)[0]
			if rec.ID != "a" || rec.CompletedAt == nil || rec.Seeding {
				t.Fatalf("persisted record = %+v", rec)
			}
			if !a.wasDropped() {
				t.Fatal("a download that is not seeded keeps its engine")
			}
		}},
		{"a finished download keeps seeding when the setting asks for it", func(t *testing.T) {
			cfg := settings.Defaults()
			cfg.SeedAfterDownload = true
			m, _ := newManagerWithSettings(t, cfg)
			w := &world{t: t, m: m, engines: map[string]*fakeTorrent{}}
			a := w.add("a")

			w.finish("a", fakeFilePath(t, 100))
			w.settle("a", StatusCompleted)
			w.m.wg.Wait()

			if !mustGet(t, m, "a").Seeding || !persistedRecords(t, m)[0].Seeding {
				t.Fatal("seeding was not recorded")
			}
			if a.wasDropped() || !a.isUploading() {
				t.Fatal("a seeding download must keep an engine that uploads")
			}
		}},
		{"a disk write error fails the download and a resume brings it back", func(t *testing.T) {
			w := newWorld(t, 1)
			a := w.add("a")

			w.m.onWriteError("a", w.m.gen)(errors.New("disk full"))
			w.settle("a", StatusFailed)
			w.m.wg.Wait()
			w.expect(map[string]Status{"a": StatusFailed})

			failed := mustGet(t, w.m, "a")
			if failed.Error != errDiskWriteFailed.Error() {
				t.Fatalf("error = %q, want %q", failed.Error, errDiskWriteFailed.Error())
			}
			if got := persistedRecords(t, w.m)[0].Error; got != errDiskWriteFailed.Error() {
				t.Fatalf("persisted error = %q", got)
			}
			if a.isDownloading() || a.isUploading() {
				t.Fatal("a failed download must stop moving data")
			}

			w.must(w.m.Resume("a"))
			w.expect(map[string]Status{"a": StatusDownloading})
			if got := mustGet(t, w.m, "a").Error; got != "" {
				t.Fatalf("error after resume = %q, want it cleared", got)
			}
			if !a.isDownloading() {
				t.Fatal("resume did not open the engine again")
			}
		}},
		{"a file missing at verification fails the download and a forced start completes it once the file is back", func(t *testing.T) {
			w := newWorld(t, 1)
			a := w.add("a")
			missing := filepath.Join(t.TempDir(), "download.bin")

			w.finish("a", missing)
			w.settle("a", StatusFailed)
			w.m.wg.Wait()
			w.expect(map[string]Status{"a": StatusFailed})
			if got := mustGet(t, w.m, "a").Error; !strings.Contains(got, errFileMissing.Error()) {
				t.Fatalf("error = %q, want it to say the file is missing", got)
			}
			if a.isDownloading() {
				t.Fatal("a failed download keeps downloading")
			}

			present := fakeFilePath(t, 100)
			a.mu.Lock()
			a.paths = []string{present}
			a.mu.Unlock()
			w.must(w.m.ForceStart("a"))
			w.expect(map[string]Status{"a": StatusDownloading})
			if mustGet(t, w.m, "a").Error != "" {
				t.Fatal("a forced start must clear the failure")
			}
			w.m.sample(w.m.ctx, time.Now())
			w.settle("a", StatusCompleted)
		}},
		{"a download in verification cannot be paused but can be removed", func(t *testing.T) {
			w := newWorld(t, 1)
			a := w.add("a")
			entered := a.blockVerify()
			job, ok := w.m.beginJob(t.Context(), "a")
			if !ok {
				t.Fatal("beginJob refused to start")
			}
			settled := make(chan struct{})
			go func() {
				defer close(settled)
				defer w.m.endJob("a")
				w.m.settleRestored(job, restoreJob{id: "a"}, a, nil)
			}()
			<-entered
			w.expect(map[string]Status{"a": StatusVerifying})

			w.refuses(w.m.Pause("a"), errUnavailable)
			w.refuses(w.m.Resume("a"), errUnavailable)
			w.refuses(w.m.ForceStart("a"), errUnavailable)
			w.must(w.m.Remove("a"))
			<-settled
			w.m.wg.Wait()
			w.expectGone("a")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}
