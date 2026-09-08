package download

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type capturedEmit struct {
	name string
	data any
}

// captureEmits swaps the package-level emit hook for one that records every
// call, since application.Get() returns nil in these tests (no live wails
// app) and the real emit would otherwise be a silent no-op. Calls in this
// file only ever drive m.sample() synchronously from the test goroutine, so
// no locking is needed around the slice.
func captureEmits(t *testing.T) *[]capturedEmit {
	t.Helper()
	captured := &[]capturedEmit{}
	original := emit
	emit = func(name string, data any) {
		*captured = append(*captured, capturedEmit{name: name, data: data})
	}
	t.Cleanup(func() { emit = original })
	return captured
}

func bigDownloadingItem(id string, fileCount int) (*Download, int64) {
	files := make([]FileState, fileCount)
	for i := range files {
		files[i] = FileState{Path: fmt.Sprintf("disc/file-%05d.bin", i), Size: 1024, Selected: true}
	}
	total := selectedTotal(files)
	d := &Download{
		ID:         id,
		Name:       id,
		Type:       TypeTorrent,
		InfoHash:   id,
		Status:     StatusDownloading,
		Total:      total,
		ETASeconds: -1,
		Files:      files,
		AddedAt:    time.Now(),
	}
	return d, total
}

// TestSampleProgressTickOmitsFiles is the audit finding's regression test:
// on a torrent with 20000 files, every 250ms progress tick used to
// re-serialize the entire Files slice (2.6 MB, measured by the auditor's
// benchmark) even though only a handful of numbers actually change. A tick
// that only moves Downloaded/speeds/etc. must emit a payload with no files
// at all, well under a kilobyte regardless of file count.
func TestSampleProgressTickOmitsFiles(t *testing.T) {
	const fileCount = 20000
	m := newTestManager(t, 1)
	d, total := bigDownloadingItem("big", fileCount)

	eng := &fakeTorrent{size: total}
	m.mu.Lock()
	m.items = append(m.items, d)
	m.engines[d.ID] = eng
	m.mu.Unlock()

	captured := captureEmits(t)

	eng.mu.Lock()
	eng.done = 1024
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	var progress *ProgressUpdate
	for _, ev := range *captured {
		switch data := ev.data.(type) {
		case Download:
			t.Fatalf("tick emitted a full snapshot with %d files via %q instead of a lightweight progress update", len(data.Files), ev.name)
		case ProgressUpdate:
			if ev.name != eventProgress {
				t.Fatalf("progress update emitted under event %q, want %q", ev.name, eventProgress)
			}
			p := data
			progress = &p
		}
	}
	if progress == nil {
		t.Fatalf("no progress update captured; events: %+v", *captured)
	}
	if progress.ID != d.ID {
		t.Fatalf("progress.ID = %q, want %q", progress.ID, d.ID)
	}
	if progress.Downloaded != 1024 {
		t.Fatalf("progress.Downloaded = %d, want 1024", progress.Downloaded)
	}

	payload, err := json.Marshal(progress)
	if err != nil {
		t.Fatalf("marshal progress update: %v", err)
	}
	const maxProgressBytes = 512
	if len(payload) > maxProgressBytes {
		t.Fatalf("progress payload is %d bytes for %d files, want under %d bytes: %s", len(payload), fileCount, maxProgressBytes, payload)
	}
	t.Logf("progress payload for %d files: %d bytes", fileCount, len(payload))

	full, err := json.Marshal(snapshot(d))
	if err != nil {
		t.Fatalf("marshal full snapshot: %v", err)
	}
	t.Logf("full snapshot payload for %d files: %d bytes", fileCount, len(full))
	if len(payload) >= len(full) {
		t.Fatalf("progress payload (%d bytes) is not smaller than the full snapshot (%d bytes)", len(payload), len(full))
	}
}

// TestSampleFullSnapshotStillReachesVerifyingTransition guards the other
// half of the trade: a status change mid-tick (download finishing and
// moving into StatusVerifying) is not a quiet numeric tick, and must still
// carry the full Download, Files included, exactly as before.
func TestSampleFullSnapshotStillReachesVerifyingTransition(t *testing.T) {
	m := newTestManager(t, 1)
	// fakeTorrent.fileBytes/filesHashed only ever report one file's worth of
	// state (see manager_test.go), so a single file is what lets
	// selectedHashed actually see everything as hashed and take the
	// verifying branch; the file count itself is not what this test is
	// about.
	d, total := bigDownloadingItem("finishing", 1)

	// finish() marks the file fully hashed and downloaded, and a real
	// on-disk file of the right size lets the spawned verify goroutine
	// succeed instead of failing, so the only two full snapshots that
	// follow are the verifying transition and the completion — no
	// unrelated eventFailed noise to filter out below.
	eng := &fakeTorrent{size: total}
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, total)}
	eng.mu.Unlock()

	m.mu.Lock()
	m.items = append(m.items, d)
	m.engines[d.ID] = eng
	m.mu.Unlock()

	captured := captureEmits(t)

	m.sample(context.Background(), time.Now())
	// The verifying transition spawns a tracked verify goroutine while
	// sample() still holds m.mu; waiting for it here (instead of asserting
	// right away) keeps every write to *captured happens-before the check
	// below, so there is nothing left running when captureEmits' cleanup
	// restores emit — synchronization by channel, not by sleeping.
	m.wg.Wait()

	if len(*captured) == 0 {
		t.Fatalf("no events captured")
	}
	// The verifying transition is emitted synchronously inside sample(),
	// before the verify goroutine is even started, so it is deterministically
	// the first event regardless of how fast that goroutine runs afterwards.
	first := (*captured)[0]
	if first.name != eventUpdated {
		t.Fatalf("verifying transition emitted under event %q, want %q", first.name, eventUpdated)
	}
	snap, ok := first.data.(Download)
	if !ok {
		t.Fatalf("verifying transition payload is %T, want Download", first.data)
	}
	if len(snap.Files) != 1 {
		t.Fatalf("verifying transition snapshot has %d files, want 1", len(snap.Files))
	}
	if snap.Status != StatusVerifying {
		t.Fatalf("verifying transition snapshot status = %s, want %s", snap.Status, StatusVerifying)
	}
}
