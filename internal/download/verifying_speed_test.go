package download

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// finishingDownload runs one tick with part of the data and a second, a second
// later, with all of it and some upload, so that the rate windows hold a real
// speed at the moment the download enters verifying.
func finishingDownload(t *testing.T, ctx context.Context, path string) (*Manager, *emitLog) {
	t.Helper()
	m := newTestManager(t, 1)
	eng := m.addTestDownload("v")
	if got := m.statusOf(t, "v"); got != StatusDownloading {
		t.Fatalf("setup: status = %s", got)
	}
	start := time.Now()
	eng.mu.Lock()
	eng.done = 10
	eng.paths = []string{path}
	eng.mu.Unlock()
	m.sample(ctx, start)

	eng.finish()
	eng.mu.Lock()
	eng.sent = 4096
	eng.mu.Unlock()
	log := recordEmits(t)
	m.sample(ctx, start.Add(time.Second))
	m.wg.Wait()
	return m, log
}

func (l *emitLog) firstUpdate(id string, status Status) (Download, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if d, ok := e.data.(Download); ok && e.name == eventUpdated && d.ID == id && d.Status == status {
			return d, true
		}
	}
	return Download{}, false
}

func assertStill(t *testing.T, what string, d Download) {
	t.Helper()
	if d.DownloadSpeed != 0 || d.UploadSpeed != 0 || d.ETASeconds != -1 {
		t.Fatalf("%s: down %d B/s, up %d B/s, eta %d; want 0, 0, -1: nothing moves while the data is checked",
			what, d.DownloadSpeed, d.UploadSpeed, d.ETASeconds)
	}
}

func TestEnteringVerifyingStopsTheSpeedsAndTheETA(t *testing.T) {
	// A cancelled context stops the check at its first step, so the download
	// stays in verifying and its snapshot can be read without a race.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m, log := finishingDownload(t, ctx, filepath.Join(t.TempDir(), "missing.bin"))

	update, ok := log.firstUpdate("v", StatusVerifying)
	if !ok {
		t.Fatalf("no download:updated with status verifying; statuses %v", log.statusesOf("v"))
	}
	assertStill(t, "the update that enters verifying", update)

	d := mustGet(t, m, "v")
	if d.Status != StatusVerifying {
		t.Fatalf("status = %s, want verifying", d.Status)
	}
	assertStill(t, "the snapshot while verifying", d)
	if d.Downloaded != 100 || d.Progress != 1 {
		t.Fatalf("downloaded %d, progress %v: the numbers of the data itself must stay", d.Downloaded, d.Progress)
	}
}

func TestVerifyingEndsAsBeforeAfterTheSpeedsStop(t *testing.T) {
	t.Run("completes", func(t *testing.T) {
		m, log := finishingDownload(t, t.Context(), fakeFilePath(t, 100))

		if _, ok := log.firstUpdate("v", StatusVerifying); !ok {
			t.Fatalf("statuses %v, want verifying first", log.statusesOf("v"))
		}
		d := mustGet(t, m, "v")
		if d.Status != StatusCompleted || d.CompletedAt == nil || d.Progress != 1 {
			t.Fatalf("download = %+v, want completed", d)
		}
		if d.DownloadSpeed != 0 || d.UploadSpeed != 0 || d.ETASeconds != 0 {
			t.Fatalf("completed download: down %d, up %d, eta %d; want 0, 0, 0", d.DownloadSpeed, d.UploadSpeed, d.ETASeconds)
		}
	})
	t.Run("fails", func(t *testing.T) {
		m, log := finishingDownload(t, t.Context(), filepath.Join(t.TempDir(), "missing.bin"))

		if _, ok := log.firstUpdate("v", StatusVerifying); !ok {
			t.Fatalf("statuses %v, want verifying first", log.statusesOf("v"))
		}
		d := mustGet(t, m, "v")
		if d.Status != StatusFailed || d.Error == "" {
			t.Fatalf("download = %+v, want failed with its reason", d)
		}
		assertStill(t, "the failed download", d)
	})
}
