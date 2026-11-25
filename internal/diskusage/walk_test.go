package diskusage

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/screenshots"
)

// blockingShots holds every walk in its screenshot step until released or
// until the walk's context ends, and says when a walk got there.
type blockingShots struct {
	*fakeShots
	calls   atomic.Int32
	entered chan time.Time
	release chan struct{}
	ended   chan error
}

func newBlockingShots(f *fakeShots) *blockingShots {
	return &blockingShots{fakeShots: f, entered: make(chan time.Time, 8), release: make(chan struct{}), ended: make(chan error, 8)}
}

func (b *blockingShots) Usage(ctx context.Context) ([]screenshots.Usage, error) {
	b.calls.Add(1)
	b.entered <- time.Now()
	select {
	case <-b.release:
		return b.fakeShots.Usage(ctx)
	case <-ctx.Done():
		b.ended <- ctx.Err()
		return nil, ctx.Err()
	}
}

// doneWatch reports the first time a caller waits on the context: Scan waits
// on it only once it is attached to a walk.
type doneWatch struct {
	context.Context
	once    sync.Once
	waiting chan struct{}
}

func newDoneWatch(parent context.Context) *doneWatch {
	return &doneWatch{Context: parent, waiting: make(chan struct{})}
}

func (d *doneWatch) Done() <-chan struct{} {
	d.once.Do(func() { close(d.waiting) })
	return d.Context.Done()
}

type scanResult struct {
	rep Report
	err error
}

func scanAsync(svc *Service, ctx context.Context) <-chan scanResult {
	out := make(chan scanResult, 1)
	go func() {
		rep, err := svc.Scan(ctx)
		out <- scanResult{rep, err}
	}()
	return out
}

const deadlock = 10 * time.Second

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(deadlock):
		t.Fatalf("%s did not happen", what)
	}
	var zero T
	return zero
}

func TestSecondScanDuringAWalkJoinsIt(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	first := scanAsync(r.svc, context.Background())
	receive(t, block.entered, "the first walk")
	watch := newDoneWatch(context.Background())
	second := scanAsync(r.svc, watch)
	receive(t, watch.waiting, "the second request waiting")
	close(block.release)

	a := receive(t, first, "the first scan")
	b := receive(t, second, "the second scan")
	if a.err != nil || b.err != nil {
		t.Fatalf("scans = %v, %v", a.err, b.err)
	}
	if n := block.calls.Load(); n != 1 {
		t.Fatalf("%d walks ran, want the second request to join the first", n)
	}
	if !a.rep.ScannedAt.Equal(b.rep.ScannedAt) {
		t.Fatalf("the two answers are different walks: %v / %v", a.rep.ScannedAt, b.rep.ScannedAt)
	}
}

func TestDeleteDuringAWalkDoesNotWaitForIt(t *testing.T) {
	r := newRig(t)
	path := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(path, "a.bin"), 4)
	r.inst.partial[path] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	scan := scanAsync(r.svc, context.Background())
	receive(t, block.entered, "the walk")
	deleted := make(chan error, 1)
	go func() { deleted <- r.svc.DeleteLeftover(context.Background(), path) }()
	if err := receive(t, deleted, "the delete during the walk"); err != nil {
		t.Fatal(err)
	}
	if exists(path) {
		t.Fatal("leftover still on disk")
	}
	close(block.release)
	got := receive(t, scan, "the scan")
	if got.err != nil {
		t.Fatalf("Scan = %v, want the report of the disk after the delete", got.err)
	}
	for _, l := range got.rep.Leftovers {
		if l.Path == path {
			t.Fatalf("the report lists the leftover the delete removed: %+v", got.rep.Leftovers)
		}
	}
}

func TestWalkRequestedDuringADeleteMeasuresWhatTheDeleteLeft(t *testing.T) {
	r := newRig(t)
	path := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(path, "a.bin"), 4)
	r.inst.partial[path] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	var scan <-chan scanResult
	old := removeAll
	t.Cleanup(func() { removeAll = old })
	removeAll = func(p string) error {
		watch := newDoneWatch(context.Background())
		scan = scanAsync(r.svc, watch)
		receive(t, watch.waiting, "the scan waiting on its walk")
		for range 100 {
			runtime.Gosched()
		}
		return old(p)
	}
	if err := r.svc.DeleteLeftover(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	got := receive(t, scan, "the scan")
	if got.err != nil {
		t.Fatal(got.err)
	}
	for _, l := range got.rep.Leftovers {
		if l.Path == path {
			t.Fatalf("a walk asked for during the delete lists what the delete removed: %+v", l)
		}
	}
}

func TestCancelScanStopsTheWalk(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	scan := scanAsync(r.svc, context.Background())
	receive(t, block.entered, "the walk")
	r.svc.CancelScan()
	if err := receive(t, block.ended, "the walk seeing the cancel"); !errors.Is(err, context.Canceled) {
		t.Fatalf("walk ended with %v", err)
	}
	if got := receive(t, scan, "the scan"); !errors.Is(got.err, errScanCancelled) {
		t.Fatalf("Scan = %v, want errScanCancelled", got.err)
	}
	if r.svc.Cached() != nil {
		t.Fatal("a cancelled walk left a report")
	}
}

func TestLeavingTheScanStopsAWalkNobodyWaitsFor(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	ctx, cancel := context.WithCancel(context.Background())
	scan := scanAsync(r.svc, ctx)
	receive(t, block.entered, "the walk")
	cancel()
	if got := receive(t, scan, "the scan"); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Scan = %v, want context.Canceled", got.err)
	}
	if err := receive(t, block.ended, "the walk seeing the cancel"); !errors.Is(err, context.Canceled) {
		t.Fatalf("walk ended with %v", err)
	}
}

func TestOneRequestLeavingKeepsTheWalkForTheOther(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	stay := scanAsync(r.svc, context.Background())
	receive(t, block.entered, "the walk")
	ctx, cancel := context.WithCancel(context.Background())
	watch := newDoneWatch(ctx)
	leave := scanAsync(r.svc, watch)
	receive(t, watch.waiting, "the second request waiting")
	cancel()
	if got := receive(t, leave, "the request that left"); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Scan = %v, want context.Canceled", got.err)
	}
	close(block.release)
	if got := receive(t, stay, "the request that stayed"); got.err != nil {
		t.Fatalf("Scan = %v, want the walk to finish for the request still waiting", got.err)
	}
	if n := block.calls.Load(); n != 1 {
		t.Fatalf("%d walks ran, want one", n)
	}
	r.svc.CancelScan()
}

func TestShutdownStopsTheWalk(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	scan := scanAsync(r.svc, context.Background())
	receive(t, block.entered, "the walk")
	stopped := make(chan error, 1)
	go func() { stopped <- r.svc.ServiceShutdown() }()
	if err := receive(t, stopped, "the shutdown"); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, block.ended, "the walk seeing the shutdown"); !errors.Is(err, context.Canceled) {
		t.Fatalf("walk ended with %v", err)
	}
	if got := receive(t, scan, "the scan"); got.err == nil {
		t.Fatal("Scan succeeded although the service shut down during its walk")
	}
	if _, err := r.svc.Scan(context.Background()); !errors.Is(err, errNotStarted) {
		t.Fatalf("Scan after shutdown = %v, want errNotStarted", err)
	}
}

func TestScannedAtIsWhenTheWalkStarted(t *testing.T) {
	r := newRig(t)
	block := newBlockingShots(r.shots)
	r.svc.deps.Screenshots = block

	before := time.Now()
	scan := scanAsync(r.svc, context.Background())
	reached := receive(t, block.entered, "the walk")
	close(block.release)
	got := receive(t, scan, "the scan")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.rep.ScannedAt.Before(before) || got.rep.ScannedAt.After(reached) {
		t.Fatalf("scannedAt %v, want between the request %v and the moment the walk reached the screenshots %v", got.rep.ScannedAt, before, reached)
	}
	if cached := r.svc.Cached(); cached == nil || !cached.ScannedAt.Equal(got.rep.ScannedAt) {
		t.Fatalf("Cached = %+v", cached)
	}
}
