package accountsync

import (
	"net/http"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	syncWait  = 2 * time.Second
	quietSpan = 150 * time.Millisecond
)

func nudgeHarness(t *testing.T, delay, gap time.Duration) (*harness, chan struct{}) {
	t.Helper()
	h := newHarness(t)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	pushed := make(chan struct{}, 16)
	echo := echoPut(http.StatusOK)
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		echo(w, req)
		select {
		case pushed <- struct{}{}:
		default:
		}
	}
	h.service.nudgeDelay = delay
	h.service.minGap = gap
	if err := h.service.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := h.service.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	})
	return h, pushed
}

func waitSync(t *testing.T, pushed chan struct{}, what string) {
	t.Helper()
	select {
	case <-pushed:
	case <-time.After(syncWait):
		t.Fatalf("no sync %s", what)
	}
}

func expectQuiet(t *testing.T, pushed chan struct{}, span time.Duration, what string) {
	t.Helper()
	select {
	case <-pushed:
		t.Fatalf("unexpected sync %s", what)
	case <-time.After(span):
	}
}

func TestNudgeSyncsWithoutWaitingForTheTicker(t *testing.T) {
	h, pushed := nudgeHarness(t, 5*time.Millisecond, 0)

	h.service.Nudge()

	waitSync(t, pushed, "after a nudge")
}

func TestNudgeCollapsesABurstIntoOneSync(t *testing.T) {
	h, pushed := nudgeHarness(t, 60*time.Millisecond, 0)

	for range 20 {
		h.service.Nudge()
	}

	waitSync(t, pushed, "after a burst of nudges")
	expectQuiet(t, pushed, quietSpan, "for the rest of the burst")
}

func TestNudgeHoldsTheMinimumGapBetweenSyncs(t *testing.T) {
	h, pushed := nudgeHarness(t, 5*time.Millisecond, 300*time.Millisecond)

	h.service.Nudge()
	waitSync(t, pushed, "after the first nudge")

	h.service.Nudge()
	expectQuiet(t, pushed, 100*time.Millisecond, "inside the minimum gap")
	waitSync(t, pushed, "once the gap expired")
}

func TestNudgeStaysQuietWhenSyncIsOff(t *testing.T) {
	h, pushed := nudgeHarness(t, 5*time.Millisecond, 0)
	h.settings.value.AccountSync = false

	h.service.Nudge()

	expectQuiet(t, pushed, quietSpan, "while account sync is disabled")
}

func TestShutdownDoesNotWaitForAPendingNudge(t *testing.T) {
	h := newHarness(t)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	pushed := make(chan struct{}, 4)
	echo := echoPut(http.StatusOK)
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		echo(w, req)
		select {
		case pushed <- struct{}{}:
		default:
		}
	}
	h.service.nudgeDelay = time.Hour
	h.service.minGap = 0
	if err := h.service.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	h.service.Nudge()

	done := make(chan error, 1)
	go func() { done <- h.service.ServiceShutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(syncWait):
		t.Fatal("ServiceShutdown blocked on a pending nudge")
	}
	expectQuiet(t, pushed, quietSpan, "after shutdown")
}
