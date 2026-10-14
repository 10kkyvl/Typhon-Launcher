package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type gateRemote struct {
	mu      sync.Mutex
	calls   int
	err     error
	gate    chan struct{}
	hold    chan struct{}
	started chan int
}

func newGateRemote() *gateRemote {
	return &gateRemote{started: make(chan int, 64)}
}

func (r *gateRemote) Browse(ctx context.Context, q GameQuery) (GamePage, error) {
	r.mu.Lock()
	r.calls++
	n := r.calls
	err := r.err
	gate := r.gate
	hold := r.hold
	r.mu.Unlock()
	r.started <- n
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return GamePage{}, ctx.Err()
		}
	}
	if hold != nil {
		<-hold
	}
	if err != nil {
		return GamePage{}, err
	}
	return GamePage{
		Items:    []Game{{ID: "srv-1", Title: fmt.Sprintf("Game v%d", n)}},
		Total:    1,
		Page:     max(q.Page, 1),
		PageSize: 20,
	}, nil
}

func (r *gateRemote) setErr(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

func (r *gateRemote) setGate(gate chan struct{}) {
	r.mu.Lock()
	r.gate = gate
	r.mu.Unlock()
}

func (r *gateRemote) setHold(hold chan struct{}) {
	r.mu.Lock()
	r.hold = hold
	r.mu.Unlock()
}

func (r *gateRemote) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type staleFixture struct {
	svc     *Service
	dir     string
	remote  *gateRemote
	clock   *fakeClock
	emitted chan PageEvent
}

func newStaleFixture(t *testing.T, started bool) *staleFixture {
	t.Helper()
	dir := t.TempDir()
	f := &staleFixture{
		svc:     mustServiceAt(t, dir),
		dir:     dir,
		remote:  newGateRemote(),
		clock:   &fakeClock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		emitted: make(chan PageEvent, 16),
	}
	f.svc.clock = f.clock.Now
	f.svc.emit = func(name string, data any) {
		if name != pageEventName {
			return
		}
		event, ok := data.(PageEvent)
		if !ok {
			return
		}
		f.emitted <- event
	}
	f.svc.SetRemoteCatalog(f.remote)
	if started {
		if err := f.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
			t.Fatalf("service startup: %v", err)
		}
		t.Cleanup(func() {
			if err := f.svc.ServiceShutdown(); err != nil {
				t.Errorf("service shutdown: %v", err)
			}
		})
	}
	return f
}

func (f *staleFixture) refreshingCount() int {
	f.svc.life.mu.Lock()
	defer f.svc.life.mu.Unlock()
	return len(f.svc.life.refreshing)
}

func (f *staleFixture) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for f.refreshingCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("background refresh did not finish")
		}
		runtime.Gosched()
	}
}

func (f *staleFixture) event(t *testing.T) PageEvent {
	t.Helper()
	select {
	case event := <-f.emitted:
		return event
	case <-time.After(10 * time.Second):
		t.Fatal("catalog:page was not emitted")
		return PageEvent{}
	}
}

func (f *staleFixture) cacheFile(key string) string {
	return filepath.Join(f.dir, "catalog-pages", key+".json")
}

func TestBrowseFirstPageIsFreshWithoutCache(t *testing.T) {
	f := newStaleFixture(t, true)
	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Stale || page.Offline {
		t.Fatalf("first page stale=%v offline=%v, want both false", page.Stale, page.Offline)
	}
	if page.Key == "" {
		t.Fatal("fresh page has no key")
	}
	if _, err := os.Stat(f.cacheFile(page.Key)); err != nil {
		t.Fatalf("page cache not written: %v", err)
	}
	if f.remote.count() != 1 {
		t.Fatalf("remote calls = %d, want 1", f.remote.count())
	}
}

func TestStalePageRefreshesInBackground(t *testing.T) {
	f := newStaleFixture(t, true)
	first, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)

	second, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Stale || second.Offline {
		t.Fatalf("second page stale=%v offline=%v, want stale only", second.Stale, second.Offline)
	}
	if second.Key != first.Key {
		t.Fatalf("key = %q, want %q", second.Key, first.Key)
	}
	if len(second.Items) != 1 || second.Items[0].Title != first.Items[0].Title {
		t.Fatalf("stale items = %+v, want the cached ones %+v", second.Items, first.Items)
	}
	if second.Snapshot == "" {
		t.Fatal("stale page lost its snapshot")
	}

	event := f.event(t)
	if event.Key != first.Key {
		t.Fatalf("event key = %q, want %q", event.Key, first.Key)
	}
	if event.Page.Stale || event.Page.Key != first.Key {
		t.Fatalf("event page stale=%v key=%q", event.Page.Stale, event.Page.Key)
	}
	if len(event.Page.Items) != 1 || event.Page.Items[0].Title != "Game v2" {
		t.Fatalf("event items = %+v, want the refreshed page", event.Page.Items)
	}
	if event.Page.Snapshot != second.Snapshot {
		t.Fatalf("event snapshot = %q, want the stale response's %q", event.Page.Snapshot, second.Snapshot)
	}
	f.waitIdle(t)
	if f.remote.count() != 2 {
		t.Fatalf("remote calls = %d, want 2", f.remote.count())
	}
}

func TestConcurrentStaleHitsStartOneRefresh(t *testing.T) {
	f := newStaleFixture(t, true)
	first, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	gate := make(chan struct{})
	f.remote.setGate(gate)

	const callers = 8
	pages := make([]GamePage, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pages[i], errs[i] = f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
		}()
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if !pages[i].Stale || pages[i].Key != first.Key {
			t.Fatalf("caller %d stale=%v key=%q", i, pages[i].Stale, pages[i].Key)
		}
	}
	if got := f.remote.count(); got != 2 {
		t.Fatalf("remote calls while refresh in flight = %d, want 2 (one sync, one refresh)", got)
	}
	close(gate)
	f.event(t)
	f.waitIdle(t)
	if got := f.remote.count(); got != 2 {
		t.Fatalf("remote calls after refresh = %d, want 2", got)
	}
}

func TestPageOlderThanWindowIsFetchedSynchronously(t *testing.T) {
	f := newStaleFixture(t, true)
	if _, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(catalogPageStaleWindow)
	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Stale {
		t.Fatal("page at the window edge was served stale")
	}
	if page.Items[0].Title != "Game v2" {
		t.Fatalf("title = %q, want the freshly fetched page", page.Items[0].Title)
	}
	if f.remote.count() != 2 {
		t.Fatalf("remote calls = %d, want 2", f.remote.count())
	}
	if len(f.emitted) != 0 {
		t.Fatal("synchronous fetch emitted a page event")
	}
}

func TestPageIsNeverStaleOutsideInitialFirstPage(t *testing.T) {
	tests := []struct {
		name    string
		query   GameQuery
		durable bool
	}{
		{name: "second page", query: GameQuery{Page: 2}, durable: true},
		{name: "pinned revision", query: GameQuery{Page: 1, Revision: 7}, durable: true},
		{name: "not durable", query: GameQuery{Page: 1}, durable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStaleFixture(t, true)
			if _, err := f.svc.browseGames(context.Background(), tt.query, true); err != nil {
				t.Fatal(err)
			}
			f.clock.Advance(time.Minute)
			page, err := f.svc.browseGames(context.Background(), tt.query, tt.durable)
			if err != nil {
				t.Fatal(err)
			}
			if page.Stale {
				t.Fatal("page was served stale")
			}
			if f.remote.count() != 2 {
				t.Fatalf("remote calls = %d, want 2", f.remote.count())
			}
			f.waitIdle(t)
			if len(f.emitted) != 0 {
				t.Fatal("page event emitted")
			}
		})
	}
}

func TestCorruptPageCacheFallsBackToFetch(t *testing.T) {
	f := newStaleFixture(t, true)
	first, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.cacheFile(first.Key), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Stale {
		t.Fatal("corrupt cache was served stale")
	}
	if f.remote.count() != 2 {
		t.Fatalf("remote calls = %d, want 2", f.remote.count())
	}
	f.clock.Advance(time.Minute)
	again, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Stale {
		t.Fatal("cache rewritten after the fetch was not used")
	}
	f.event(t)
}

func TestFailedBackgroundRefreshEmitsNothingAndCanRetry(t *testing.T) {
	f := newStaleFixture(t, true)
	if _, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	f.remote.setErr(errors.New("backend down"))

	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Stale || page.Offline {
		t.Fatalf("stale=%v offline=%v, want stale only", page.Stale, page.Offline)
	}
	f.waitIdle(t)
	if len(f.emitted) != 0 {
		t.Fatal("failed refresh emitted an event")
	}
	if f.remote.count() != 2 {
		t.Fatalf("remote calls = %d, want 2", f.remote.count())
	}

	f.remote.setErr(nil)
	if _, err = f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	event := f.event(t)
	if event.Page.Items[0].Title != "Game v3" {
		t.Fatalf("retried refresh title = %q, want Game v3", event.Page.Items[0].Title)
	}
}

func TestStaleNeedsStartedService(t *testing.T) {
	f := newStaleFixture(t, false)
	if _, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Stale {
		t.Fatal("service without a lifetime served a stale page")
	}
	if f.remote.count() != 2 {
		t.Fatalf("remote calls = %d, want 2", f.remote.count())
	}
	if len(f.emitted) != 0 {
		t.Fatal("page event emitted without a started service")
	}
}

func TestShutdownWaitsForRefreshAndSuppressesEvent(t *testing.T) {
	f := newStaleFixture(t, true)
	if _, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	hold := make(chan struct{})
	f.remote.setHold(hold)
	if page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil || !page.Stale {
		t.Fatalf("stale hit: page.Stale=%v err=%v", page.Stale, err)
	}
	for n := range f.remote.started {
		if n == 2 {
			break
		}
	}

	done := make(chan error, 1)
	go func() { done <- f.svc.ServiceShutdown() }()
	select {
	case err := <-done:
		t.Fatalf("shutdown returned before the refresh finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if len(f.emitted) != 0 {
		t.Fatal("event emitted after shutdown")
	}
	if n := f.refreshingCount(); n != 0 {
		t.Fatalf("refreshing keys after shutdown = %d, want 0", n)
	}
	if f.svc.startRefresh("again", func(context.Context) {}) {
		t.Fatal("refresh started after shutdown")
	}
}

func TestOfflineCacheStillServedWhenBackendFailsSynchronously(t *testing.T) {
	f := newStaleFixture(t, true)
	if _, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(catalogPageStaleWindow + time.Minute)
	f.remote.setErr(errors.New("backend down"))
	page, err := f.svc.BrowseGames(context.Background(), GameQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Offline || page.Stale || page.Key == "" {
		t.Fatalf("offline=%v stale=%v key=%q, want offline fallback with key", page.Offline, page.Stale, page.Key)
	}
}
