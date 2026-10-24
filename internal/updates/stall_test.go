package updates

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"typhon/internal/download"
)

type stepClock struct {
	mu    sync.Mutex
	at    time.Time
	ticks chan time.Time
}

func newStepClock() *stepClock {
	return &stepClock{at: time.Unix(1000, 0), ticks: make(chan time.Time)}
}

func (c *stepClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(d)
	c.mu.Unlock()
}

func (c *stepClock) poll() (<-chan time.Time, func()) {
	return c.ticks, func() {}
}

func (c *stepClock) install(svc *Service) {
	svc.now = c.now
	svc.poll = c.poll
}

func TestWaitDownloadStallTimeoutFiresWhenProgressFreezes(t *testing.T) {
	previousStall := updateStallTimeout
	updateStallTimeout = 30 * time.Second
	t.Cleanup(func() { updateStallTimeout = previousStall })

	downloads := newFakeDownloads()
	downloads.tasks["d1"] = &download.Download{ID: "d1", Status: download.StatusDownloading, Downloaded: 100}

	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.downloads = downloads
	svc.updates["g1"] = &Update{GameID: "g1"}
	clock := newStepClock()
	clock.install(svc)

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			clock.advance(10 * time.Second)
			select {
			case clock.ticks <- clock.now():
			case <-done:
				return
			}
		}
	}()

	err = svc.waitDownload(context.Background(), "g1", "d1")
	if !errors.Is(err, errDownloadStalled) {
		t.Fatalf("err = %v, want errDownloadStalled", err)
	}
}

func TestWaitDownloadStallTimeoutResetsOnProgress(t *testing.T) {
	previousStall := updateStallTimeout
	updateStallTimeout = 40 * time.Second
	t.Cleanup(func() { updateStallTimeout = previousStall })

	downloads := newFakeDownloads()
	downloads.tasks["d1"] = &download.Download{ID: "d1", Status: download.StatusDownloading, Downloaded: 100}

	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.downloads = downloads
	svc.updates["g1"] = &Update{GameID: "g1"}
	clock := newStepClock()
	clock.install(svc)

	done := make(chan struct{})
	defer close(done)
	go func() {
		downloaded := int64(100)
		clock.ticks <- clock.now()
		for step := 1; step <= 4; step++ {
			downloaded += 50
			downloads.mu.Lock()
			downloads.tasks["d1"].Downloaded = downloaded
			if step == 4 {
				downloads.tasks["d1"].Status = download.StatusCompleted
			}
			downloads.mu.Unlock()
			clock.advance(30 * time.Second)
			select {
			case clock.ticks <- clock.now():
			case <-done:
				return
			}
		}
	}()

	if err := svc.waitDownload(context.Background(), "g1", "d1"); err != nil {
		t.Fatalf("waitDownload: %v, want nil: progress should keep resetting the stall clock", err)
	}
}

func TestWaitDownloadFailsWithoutStallingOnDownloadsError(t *testing.T) {
	previousPoll := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previousPoll })

	downloads := newFakeDownloads()
	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.downloads = downloads
	svc.updates["g1"] = &Update{GameID: "g1"}

	if err := svc.waitDownload(context.Background(), "g1", "missing"); !errors.Is(err, errDownloadFailed) {
		t.Fatalf("err = %v, want errDownloadFailed", err)
	}
}
