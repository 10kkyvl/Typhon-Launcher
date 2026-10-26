package achievements

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type recordedReport struct {
	gameID, appID string
	unlocks       []Unlock
}

type fakeReporter struct {
	mu    sync.Mutex
	calls []recordedReport
	err   error
}

func (f *fakeReporter) Report(gameID, appID string, unlocks []Unlock) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedReport{gameID, appID, slices.Clone(unlocks)})
	return f.err
}

func TestScanReportsOnlyFreshUnlocksNotTheBaseline(t *testing.T) {
	h := newHarness(t)
	rep := &fakeReporter{}
	h.svc.reporter = rep
	ctx := context.Background()

	h.writeProgress(t, `{"ACH_A":{"earned":true,"earned_time":1760000000}}`)
	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if len(rep.calls) != 0 {
		t.Fatalf("the baseline was reported: %+v", rep.calls)
	}

	h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true,"earned_time":1760000500}}`)
	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if len(rep.calls) != 1 {
		t.Fatalf("calls = %+v", rep.calls)
	}
	call := rep.calls[0]
	if call.gameID != "g1" || call.appID != "620" || len(call.unlocks) != 1 || call.unlocks[0].Key != "ACH_B" {
		t.Fatalf("call = %+v", call)
	}
	if want := time.Unix(1760000500, 0); !call.unlocks[0].At.Equal(want) {
		t.Fatalf("unlock time = %v, want %v", call.unlocks[0].At, want)
	}

	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatalf("repeat scan: %v", err)
	}
	if len(rep.calls) != 1 {
		t.Fatalf("a repeated scan reported again: %+v", rep.calls)
	}
}

func TestReporterFailureDoesNotFailTheScan(t *testing.T) {
	h := newHarness(t)
	h.svc.reporter = &fakeReporter{err: errors.New("disk full")}
	ctx := context.Background()
	h.writeProgress(t, `{"ACH_A":{"earned":true}}`)
	if _, err := h.svc.Scan(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	h.writeProgress(t, `{"ACH_A":{"earned":true},"ACH_B":{"earned":true}}`)
	prog, err := h.svc.Scan(ctx, "g1")
	if err != nil || prog.Unlocked != 2 {
		t.Fatalf("scan = %+v, %v", prog, err)
	}
	if got := h.events.snapshot(); len(got) != 1 {
		t.Fatalf("the announcement was lost: %+v", got)
	}
}
