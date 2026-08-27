package updates

import (
	"context"
	"errors"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/usagestats"
)

type usageRecorder struct {
	mu     sync.Mutex
	events []usagestats.Event
}

func (r *usageRecorder) record(ev usagestats.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *usageRecorder) snapshot() []usagestats.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]usagestats.Event(nil), r.events...)
}

// blockingDownloads blocks AddTask until ctx is cancelled, so a test can
// exercise the mid-flight CancelUpdate path deterministically instead of
// racing the instant fakeDownloads.
type blockingDownloads struct{ *fakeDownloads }

func (b *blockingDownloads) AddTask(ctx context.Context, _ download.AddRequest) (download.Download, error) {
	<-ctx.Done()
	return download.Download{}, ctx.Err()
}

func eventTypes(events []usagestats.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Type
	}
	return out
}

// TestStartUpdateRecordsUsageLifecycle catches an update that never reports
// its outcome to usagestats: before the fix, apply.go emitted eventCompleted
// / eventFailed for the UI but never called into usagestats at all, so this
// test failed with "events = [update_started], want 2 events".
func TestStartUpdateRecordsUsageLifecycle(t *testing.T) {
	cases := []struct {
		name     string
		failTask bool
		wantType string
	}{
		{name: "completed", failTask: false, wantType: usagestats.TypeUpdateCompleted},
		{name: "failed", failTask: true, wantType: usagestats.TypeUpdateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			rec := &usageRecorder{}
			h.service.SetUsageRecorder(rec.record)
			h.plan(t)
			h.downloads.failTask = tc.failTask

			if err := h.service.StartUpdate("local-1"); err != nil {
				t.Fatalf("start update: %v", err)
			}
			h.awaitJob(t, "local-1")

			events := rec.snapshot()
			if len(events) != 2 {
				t.Fatalf("events = %v, want 2 events", eventTypes(events))
			}
			if events[0].Type != usagestats.TypeUpdateStarted {
				t.Fatalf("events[0].Type = %q, want %q", events[0].Type, usagestats.TypeUpdateStarted)
			}
			if events[0].Properties.GameID != canonical {
				t.Fatalf("started GameID = %q, want %q", events[0].Properties.GameID, canonical)
			}
			terminal := events[1]
			if terminal.Type != tc.wantType {
				t.Fatalf("events[1].Type = %q, want %q", terminal.Type, tc.wantType)
			}
			if terminal.Properties.GameID != canonical {
				t.Fatalf("terminal GameID = %q, want %q", terminal.Properties.GameID, canonical)
			}
			if terminal.Properties.DurationSeconds < 0 {
				t.Fatalf("negative duration: %d", terminal.Properties.DurationSeconds)
			}
			if tc.failTask {
				if terminal.Properties.ErrorCode != usagestats.CodeUnknown {
					t.Fatalf("error code = %q, want %q", terminal.Properties.ErrorCode, usagestats.CodeUnknown)
				}
				if terminal.Properties.ErrorCode == "сеть недоступна" {
					t.Fatal("raw error text leaked into ErrorCode")
				}
			}
		})
	}
}

// TestUpdateDownloadOriginCarriesCanonicalGameID catches a join-key gap: the
// download.Origin passed to download.Manager.AddTask must carry the same
// canonical GameID as the update_* usagestats events for the same operation,
// or download_started/completed/failed events for update downloads land with
// an empty game_id and can never be correlated with their update_* siblings.
// Before the fix, downloadRelease only set Origin.LibraryID (the local
// library id), never Origin.GameID.
func TestUpdateDownloadOriginCarriesCanonicalGameID(t *testing.T) {
	h := newHarness(t)
	h.plan(t)

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatalf("start update: %v", err)
	}
	h.awaitJob(t, "local-1")

	if len(h.downloads.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(h.downloads.requests))
	}
	if got := h.downloads.requests[0].Origin.GameID; got != canonical {
		t.Fatalf("update download Origin.GameID = %q, want %q", got, canonical)
	}
}

// TestRepairDownloadOriginCarriesCanonicalGameID mirrors the update case for
// repair: the repair download's Origin.GameID must match the canonical id
// used in repair_* usagestats events.
func TestRepairDownloadOriginCarriesCanonicalGameID(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(damagedTorrentReport())
	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	if err := h.service.RepairGame("local-1"); err != nil {
		t.Fatalf("repair: %v", err)
	}
	h.awaitJob(t, "local-1")

	if len(h.downloads.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(h.downloads.requests))
	}
	if got := h.downloads.requests[0].Origin.GameID; got != canonical {
		t.Fatalf("repair download Origin.GameID = %q, want %q", got, canonical)
	}
}

// TestCancelUpdateRecordsCancelledOutcome catches a cancellation that leaves
// update_started with no terminal event: started counts would then exceed the
// sum of outcomes forever, with no way to tell why. A cancel is reported as
// TypeUpdateFailed carrying error_code "cancelled", which keeps it countable
// and still distinguishable from a genuine failure.
func TestCancelUpdateRecordsCancelledOutcome(t *testing.T) {
	h := newHarness(t)
	rec := &usageRecorder{}
	h.service.SetUsageRecorder(rec.record)
	h.plan(t)
	h.service.downloads = &blockingDownloads{h.downloads}

	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatalf("start update: %v", err)
	}
	if err := h.service.CancelUpdate("local-1"); err != nil {
		t.Fatalf("cancel update: %v", err)
	}
	h.awaitJob(t, "local-1")

	events := rec.snapshot()
	if len(events) != 2 {
		t.Fatalf("events = %v, want started followed by a terminal event", eventTypes(events))
	}
	if events[0].Type != usagestats.TypeUpdateStarted {
		t.Fatalf("first event = %q, want %q", events[0].Type, usagestats.TypeUpdateStarted)
	}
	if events[1].Type != usagestats.TypeUpdateFailed {
		t.Fatalf("terminal event = %q, want %q", events[1].Type, usagestats.TypeUpdateFailed)
	}
	if got := events[1].Properties.ErrorCode; got != usagestats.CodeCancelled {
		t.Fatalf("error code = %q, want %q so a cancel is not counted as a real failure", got, usagestats.CodeCancelled)
	}
}

func damagedTorrentReport() download.ReuseReport {
	return download.ReuseReport{
		InfoHash:     "hash-r1",
		Layout:       download.LayoutDirectFiles,
		Applicable:   true,
		PresentFiles: 3,
		Flat:         true,
		TotalBytes:   1000,
		MatchedBytes: 700,
		MissingBytes: 300,
		MissingFiles: 1,
		TotalPieces:  10,
		OkPieces:     7,
		BadPieces:    3,
	}
}

// TestVerifyGameRecordsUsageLifecycle catches a verification whose outcome
// never reaches usagestats: before the fix this failed with only a started
// event captured, regardless of how the run actually ended.
func TestVerifyGameRecordsUsageLifecycle(t *testing.T) {
	t.Run("completed via manifest", func(t *testing.T) {
		h := newHarness(t)
		rec := &usageRecorder{}
		h.service.SetUsageRecorder(rec.record)
		h.seedManifest(t)

		if err := h.service.VerifyGame("local-1"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		h.awaitJob(t, "local-1")

		events := rec.snapshot()
		if len(events) != 2 {
			t.Fatalf("events = %v, want 2 events", eventTypes(events))
		}
		if events[0].Type != usagestats.TypeVerifyStarted || events[0].Properties.GameID != canonical {
			t.Fatalf("started event = %+v", events[0])
		}
		completed := events[1]
		if completed.Type != usagestats.TypeVerifyCompleted {
			t.Fatalf("events[1].Type = %q, want %q", completed.Type, usagestats.TypeVerifyCompleted)
		}
		if completed.Properties.GameID != canonical || completed.Properties.DurationSeconds < 0 {
			t.Fatalf("completed event = %+v", completed)
		}
	})

	t.Run("failed via torrent inspection error", func(t *testing.T) {
		h := newHarness(t)
		rec := &usageRecorder{}
		h.service.SetUsageRecorder(rec.record)
		// No withTorrent call: fakeDownloads keeps its default reuseErr, and
		// only the release's InfoHash is set so torrentIdentity resolves,
		// hasManifest stays false, landing in the genuine-failure branch of
		// verify() rather than the cancellation one.
		h.releases.list[0].InfoHash = "hash-r1"

		if err := h.service.VerifyGame("local-1"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		h.awaitJob(t, "local-1")

		events := rec.snapshot()
		if len(events) != 2 {
			t.Fatalf("events = %v, want 2 events", eventTypes(events))
		}
		failed := events[1]
		if failed.Type != usagestats.TypeVerifyFailed {
			t.Fatalf("events[1].Type = %q, want %q", failed.Type, usagestats.TypeVerifyFailed)
		}
		if failed.Properties.GameID != canonical {
			t.Fatalf("failed GameID = %q, want %q", failed.Properties.GameID, canonical)
		}
		if failed.Properties.ErrorCode != usagestats.CodeUnknown {
			t.Fatalf("error code = %q, want %q", failed.Properties.ErrorCode, usagestats.CodeUnknown)
		}
		if failed.Properties.ErrorCode == "недоступно" {
			t.Fatal("raw error text leaked into ErrorCode")
		}
	})
}

// TestVerifyGameRecordsUsageForUnavailableOutcome catches the third terminal
// branch of verify() (torrent present but doesn't describe the install, and
// no manifest to fall back to): before the fix, this path only emitted the
// UI event and never reached usagestats, leaving a started event with no
// paired terminal event.
func TestVerifyGameRecordsUsageForUnavailableOutcome(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(download.ReuseReport{Applicable: false})
	rec := &usageRecorder{}
	h.service.SetUsageRecorder(rec.record)

	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	events := rec.snapshot()
	if len(events) != 2 {
		t.Fatalf("events = %v, want 2 events", eventTypes(events))
	}
	if events[0].Type != usagestats.TypeVerifyStarted || events[0].Properties.GameID != canonical {
		t.Fatalf("started event = %+v", events[0])
	}
	completed := events[1]
	if completed.Type != usagestats.TypeVerifyCompleted {
		t.Fatalf("events[1].Type = %q, want %q", completed.Type, usagestats.TypeVerifyCompleted)
	}
	if completed.Properties.GameID != canonical || completed.Properties.DurationSeconds < 0 {
		t.Fatalf("completed event = %+v", completed)
	}
}

// TestRepairGameRecordsUsageLifecycle mirrors the verify case for repair:
// before the fix, RepairGame's terminal states never reached usagestats.
func TestRepairGameRecordsUsageLifecycle(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		h := newHarness(t)
		h.withTorrent(damagedTorrentReport())
		if err := h.service.VerifyGame("local-1"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		h.awaitJob(t, "local-1")

		rec := &usageRecorder{}
		h.service.SetUsageRecorder(rec.record)
		if err := h.service.RepairGame("local-1"); err != nil {
			t.Fatalf("repair: %v", err)
		}
		h.awaitJob(t, "local-1")

		events := rec.snapshot()
		if len(events) != 2 {
			t.Fatalf("events = %v, want 2 events", eventTypes(events))
		}
		if events[0].Type != usagestats.TypeRepairStarted || events[0].Properties.GameID != canonical {
			t.Fatalf("started event = %+v", events[0])
		}
		completed := events[1]
		if completed.Type != usagestats.TypeRepairCompleted {
			t.Fatalf("events[1].Type = %q, want %q", completed.Type, usagestats.TypeRepairCompleted)
		}
		if completed.Properties.GameID != canonical || completed.Properties.DurationSeconds < 0 {
			t.Fatalf("completed event = %+v", completed)
		}
	})

	t.Run("failed", func(t *testing.T) {
		h := newHarness(t)
		h.withTorrent(damagedTorrentReport())
		if err := h.service.VerifyGame("local-1"); err != nil {
			t.Fatalf("verify: %v", err)
		}
		h.awaitJob(t, "local-1")

		rec := &usageRecorder{}
		h.service.SetUsageRecorder(rec.record)
		h.downloads.addErr = errors.New("не удалось создать задачу")
		if err := h.service.RepairGame("local-1"); err != nil {
			t.Fatalf("repair: %v", err)
		}
		h.awaitJob(t, "local-1")

		events := rec.snapshot()
		if len(events) != 2 {
			t.Fatalf("events = %v, want 2 events", eventTypes(events))
		}
		failed := events[1]
		if failed.Type != usagestats.TypeRepairFailed {
			t.Fatalf("events[1].Type = %q, want %q", failed.Type, usagestats.TypeRepairFailed)
		}
		if failed.Properties.GameID != canonical {
			t.Fatalf("failed GameID = %q, want %q", failed.Properties.GameID, canonical)
		}
		if failed.Properties.ErrorCode != usagestats.CodeUnknown {
			t.Fatalf("error code = %q, want %q", failed.Properties.ErrorCode, usagestats.CodeUnknown)
		}
		if failed.Properties.ErrorCode == "не удалось создать задачу" {
			t.Fatal("raw error text leaked into ErrorCode")
		}
	})
}
