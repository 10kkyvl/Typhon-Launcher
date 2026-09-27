package updates

import (
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/download"
)

func TestWaitDownloadStallTimeoutFiresWhenProgressFreezes(t *testing.T) {
	previousPoll := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previousPoll })
	previousStall := updateStallTimeout
	updateStallTimeout = 30 * time.Millisecond
	t.Cleanup(func() { updateStallTimeout = previousStall })

	downloads := newFakeDownloads()
	downloads.tasks["d1"] = &download.Download{ID: "d1", Status: download.StatusDownloading, Downloaded: 100}

	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.downloads = downloads
	svc.updates["g1"] = &Update{GameID: "g1"}

	err = svc.waitDownload(context.Background(), "g1", "d1")
	if !errors.Is(err, errDownloadStalled) {
		t.Fatalf("err = %v, want errDownloadStalled", err)
	}
}

func TestWaitDownloadStallTimeoutResetsOnProgress(t *testing.T) {
	previousPoll := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = previousPoll })
	previousStall := updateStallTimeout
	updateStallTimeout = 40 * time.Millisecond
	t.Cleanup(func() { updateStallTimeout = previousStall })

	downloads := newFakeDownloads()
	downloads.tasks["d1"] = &download.Download{ID: "d1", Status: download.StatusDownloading, Downloaded: 100}

	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.downloads = downloads
	svc.updates["g1"] = &Update{GameID: "g1"}

	stop := make(chan struct{})
	defer close(stop)
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		downloaded := int64(100)
		ticks := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				ticks++
				downloaded += 50
				downloads.mu.Lock()
				downloads.tasks["d1"].Downloaded = downloaded
				if ticks >= 4 {
					downloads.tasks["d1"].Status = download.StatusCompleted
				}
				downloads.mu.Unlock()
				if ticks >= 4 {
					return
				}
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
