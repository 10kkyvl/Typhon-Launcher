package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/usagestats"
)

func TestUsageCompletedDurationAndAverageSpeed(t *testing.T) {
	cases := []struct {
		name         string
		durationSecs int
		wantAvgSpeed bool
	}{
		{"positive duration computes average speed", 10, true},
		{"zero duration leaves average speed unset", 0, false},
		{"negative duration leaves average speed unset", -5, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 2)
			d := m.addTestItem("a", StatusDownloading)

			var got usagestats.Event
			m.SetUsageRecorder(func(ev usagestats.Event) { got = ev })

			m.mu.Lock()
			d.Origin.GameID = "game-1"
			d.Downloaded = 100
			d.AddedAt = time.Now().Add(-time.Duration(c.durationSecs) * time.Second)
			m.completeLocked(d)
			m.mu.Unlock()

			if got.Type != usagestats.TypeDownloadCompleted {
				t.Fatalf("type = %q, want %q", got.Type, usagestats.TypeDownloadCompleted)
			}
			if got.Properties.GameID != "game-1" {
				t.Fatalf("game id = %q", got.Properties.GameID)
			}
			if got.Properties.BytesTotal != d.Total {
				t.Fatalf("bytes total = %d, want %d", got.Properties.BytesTotal, d.Total)
			}
			hasAvg := got.Properties.AverageSpeedBytes != 0
			if hasAvg != c.wantAvgSpeed {
				t.Fatalf("average speed = %d, want non-zero=%v", got.Properties.AverageSpeedBytes, c.wantAvgSpeed)
			}
		})
	}
}

func TestUsageIntegrationRecordsCompletedThroughSample(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	m.mu.Lock()
	m.findLocked("a").Origin.GameID = "game-2"
	m.mu.Unlock()

	got := make(chan usagestats.Event, 1)
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type == usagestats.TypeDownloadCompleted {
			got <- ev
		}
	})

	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{fakeFilePath(t, eng.size)}
	eng.mu.Unlock()
	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to complete", func() bool { return m.statusOf(t, "a") == StatusCompleted })

	select {
	case ev := <-got:
		if ev.Properties.GameID != "game-2" {
			t.Fatalf("game id = %q", ev.Properties.GameID)
		}
		if ev.Properties.BytesTotal != 100 {
			t.Fatalf("bytes total = %d, want 100", ev.Properties.BytesTotal)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completed event never recorded through the real sample/verify pipeline")
	}
}

func TestUsageRecordsDownloadFailedWithoutLeakingMessage(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusQueued)
	m.mu.Lock()
	d.Origin.GameID = "game-77"
	d.AddedAt = time.Now().Add(-3 * time.Second)
	d.Downloaded = 42
	d.Total = 4200
	m.mu.Unlock()

	var got usagestats.Event
	recorded := make(chan struct{}, 1)
	m.SetUsageRecorder(func(ev usagestats.Event) {
		got = ev
		recorded <- struct{}{}
	})

	const sensitivePath = `D:\Users\egor\Downloads\Startup Panic [ABCDEF0123456789].torrent`
	message := "не удалось прочитать " + sensitivePath
	cause := fmt.Errorf("stat %s: отказано в доступе: %w", sensitivePath, os.ErrPermission)

	m.markFailed("a", message, cause)

	select {
	case <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("failed event not recorded")
	}

	if got.Type != usagestats.TypeDownloadFailed {
		t.Fatalf("type = %q, want %q", got.Type, usagestats.TypeDownloadFailed)
	}
	wantCode := usagestats.Classify(cause)
	if got.Properties.ErrorCode != wantCode {
		t.Fatalf("error code = %q, want %q", got.Properties.ErrorCode, wantCode)
	}
	if got.Properties.GameID != "game-77" {
		t.Fatalf("game id = %q", got.Properties.GameID)
	}
	if got.Properties.BytesTotal != 4200 {
		t.Fatalf("bytes total = %d, want 4200 (the download size, not the bytes transferred)", got.Properties.BytesTotal)
	}

	dirty := []string{message, sensitivePath, "Startup Panic", "ABCDEF0123456789", "отказано", "egor"}
	fields := []string{got.Properties.GameID, got.Properties.InstallerType, got.Properties.ErrorCode, got.Type}
	for _, field := range fields {
		for _, leak := range dirty {
			if leak != "" && strings.Contains(field, leak) {
				t.Fatalf("event field %q leaks %q from the raw error/message", field, leak)
			}
		}
	}
}

func TestUsageFailWithoutClientRecordsFailed(t *testing.T) {
	m := newTestManager(t, 2)
	d := m.addTestItem("a", StatusQueued)
	m.mu.Lock()
	d.Origin.GameID = "game-3"
	d.AddedAt = time.Now().Add(-2 * time.Second)
	d.Downloaded = 10
	d.Total = 1000
	m.mu.Unlock()

	var got usagestats.Event
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type == usagestats.TypeDownloadFailed {
			got = ev
		}
	})

	m.failWithoutClient()

	if got.Type != usagestats.TypeDownloadFailed {
		t.Fatal("failWithoutClient did not record a failed event")
	}
	if got.Properties.GameID != "game-3" {
		t.Fatalf("game id = %q", got.Properties.GameID)
	}
	if got.Properties.BytesTotal != 1000 {
		t.Fatalf("bytes total = %d, want 1000 (the download size, not the bytes transferred)", got.Properties.BytesTotal)
	}
	wantCode := usagestats.Classify(errNoClient)
	if got.Properties.ErrorCode != wantCode {
		t.Fatalf("error code = %q, want %q", got.Properties.ErrorCode, wantCode)
	}
}

func TestUsageRecordsDownloadCancelledBeforeDrop(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	m.mu.Lock()
	d := m.findLocked("a")
	d.Origin.GameID = "game-9"
	d.AddedAt = time.Now().Add(-5 * time.Second)
	d.Downloaded = 55
	d.Total = 5500
	m.mu.Unlock()

	var got usagestats.Event
	recorded := make(chan struct{}, 1)
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type == usagestats.TypeDownloadCancelled {
			got = ev
			recorded <- struct{}{}
		}
	})

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled event not recorded")
	}
	waitUntil(t, "torrent to be dropped", eng.wasDropped)

	if got.Properties.GameID != "game-9" {
		t.Fatalf("game id = %q", got.Properties.GameID)
	}
	if got.Properties.BytesTotal != 5500 {
		t.Fatalf("bytes total = %d, want 5500 (download size, captured before dropLocked)", got.Properties.BytesTotal)
	}
	if _, err := m.Get("a"); !errors.Is(err, errNotFound) {
		t.Fatalf("get after cancel = %v, want %v", err, errNotFound)
	}
}

func TestUsageRemoveDoesNotRecordCancelled(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestDownload("a")

	var mu sync.Mutex
	cancelledEvents := 0
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type == usagestats.TypeDownloadCancelled {
			mu.Lock()
			cancelledEvents++
			mu.Unlock()
		}
	})

	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if cancelledEvents != 0 {
		t.Fatalf("cancelled events recorded by Remove = %d, want 0", cancelledEvents)
	}
}

func TestUsageCancelAfterCompletionDoesNotRecordCancelled(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestDownload("a")
	m.mu.Lock()
	m.findLocked("a").Status = StatusCompleted
	m.mu.Unlock()

	var mu sync.Mutex
	cancelledEvents := 0
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type == usagestats.TypeDownloadCancelled {
			mu.Lock()
			cancelledEvents++
			mu.Unlock()
		}
	})

	if err := m.Cancel("a"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if cancelledEvents != 0 {
		t.Fatalf("cancelled events for an already completed download = %d, want 0 (download_completed already reported it)", cancelledEvents)
	}
}

func TestUsageNilRecorderDoesNotPanic(t *testing.T) {
	m := newTestManager(t, 2)

	m.markFailed("missing", "boom", errors.New("boom cause"))

	m.addTestDownload("a")
	m.markFailed("a", "write failed", errors.New("disk full"))

	b := m.addTestDownload("b")
	b.finish()
	b.mu.Lock()
	b.paths = []string{fakeFilePath(t, b.size)}
	b.mu.Unlock()
	m.sample(context.Background(), time.Now())
	waitUntil(t, "b to complete", func() bool { return m.statusOf(t, "b") == StatusCompleted })

	m.addTestDownload("c")
	if err := m.Cancel("c"); err != nil {
		t.Fatal(err)
	}

	m.failWithoutClient()
}

func TestUsageRecorderRaceUnderConcurrentFailures(t *testing.T) {
	m := newTestManager(t, 8)
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, id := range ids {
		m.addTestDownload(id)
	}

	var mu sync.Mutex
	count := 0
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type != usagestats.TypeDownloadFailed {
			return
		}
		mu.Lock()
		count++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			m.markFailed(id, "concurrent failure", errors.New("boom"))
		}(id)
	}
	wg.Wait()

	for _, id := range ids {
		if got := m.statusOf(t, id); got != StatusFailed {
			t.Fatalf("%s status = %s, want %s", id, got, StatusFailed)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if count != len(ids) {
		t.Fatalf("recorded %d failures, want %d", count, len(ids))
	}
}

func TestUsageRecorderRaceUnderConcurrentCancel(t *testing.T) {
	m := newTestManager(t, 8)
	ids := []string{"a", "b", "c", "d"}
	for _, id := range ids {
		m.addTestDownload(id)
	}

	var mu sync.Mutex
	cancelled := 0
	m.SetUsageRecorder(func(ev usagestats.Event) {
		if ev.Type != usagestats.TypeDownloadCancelled {
			return
		}
		mu.Lock()
		cancelled++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := m.Cancel(id); err != nil {
				t.Errorf("cancel %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if cancelled != len(ids) {
		t.Fatalf("cancelled events = %d, want %d", cancelled, len(ids))
	}
}
