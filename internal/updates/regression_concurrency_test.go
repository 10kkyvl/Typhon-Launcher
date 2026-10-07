package updates

import (
	"errors"
	"sync"
	"testing"
)

func TestConcurrentStartUpdateRunsOnce(t *testing.T) {
	h := newHarness(t)
	hanging := &hangingDownloads{fakeDownloads: h.downloads, hang: "r2"}
	h.service.downloads = hanging
	h.plan(t)

	const callers = 8
	results := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	gate := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-gate
			results <- h.service.StartUpdate("local-1")
		}()
	}
	ready.Wait()
	close(gate)

	started, busy := 0, 0
	for range callers {
		switch err := <-results; {
		case err == nil:
			started++
		case errors.Is(err, errBusy):
			busy++
		default:
			t.Errorf("StartUpdate returned %v, want success or %v", err, errBusy)
		}
	}
	if started != 1 || busy != callers-1 {
		t.Fatalf("started = %d busy = %d, want exactly one update and %d refusals", started, busy, callers-1)
	}

	waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.DownloadID != "" })
	if history := h.service.GetHistory(""); len(history) != 1 {
		t.Fatalf("history = %+v, want one entry for one update", history)
	}
	h.downloads.mu.Lock()
	requests := len(h.downloads.requests)
	h.downloads.mu.Unlock()
	if requests != 1 {
		t.Fatalf("download requests = %d, want one", requests)
	}

	if err := h.service.CancelUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.State == StateAvailable && !h.service.Busy("local-1") })
}

func TestStateReadersDoNotRaceWithARunningUpdate(t *testing.T) {
	h := newHarness(t)
	hanging := &hangingDownloads{fakeDownloads: h.downloads, hang: "r2"}
	h.service.downloads = hanging
	h.plan(t)
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.DownloadID != "" })

	const workers = 6
	const rounds = 30
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				switch worker % 3 {
				case 0:
					h.service.GetUpdates()
					h.service.GetHistory("local-1")
					h.service.Busy("local-1")
					h.service.HasRollback("local-1")
				case 1:
					if _, err := h.service.CheckGame("local-1"); err != nil {
						t.Errorf("CheckGame: %v", err)
						return
					}
					h.service.HandleSessionEnded("local-1", 1)
				default:
					h.service.sweepPrevious()
					h.service.prune(map[string]bool{"local-1": true})
					h.service.appendHistory(UpdateHistory{ID: "noise", GameID: "other"})
				}
			}
		}()
	}
	wg.Wait()

	got, _ := h.service.snapshot("local-1")
	if got.State != StateUpdating {
		t.Fatalf("state = %q, want the running update to be undisturbed by the readers", got.State)
	}
	if err := h.service.CancelUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.State == StateAvailable && !h.service.Busy("local-1") })
}
