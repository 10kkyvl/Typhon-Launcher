package sources

import (
	"net/http"
	"testing"
	"time"

	"typhon/internal/sources/feed"
)

// TestServiceShutdownWaitsForNotifyChangedGoroutine covers invariant 19:
// notifyChanged used to start "go notify()" without registering it in s.wg,
// so ServiceShutdown's wg.Wait() could return before a slow onChanged
// callback (updates.HandleSourcesRefreshed walks every installed game)
// finished. Blocking the callback lets the test see whether Shutdown waits.
func TestServiceShutdownWaitsForNotifyChangedGoroutine(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, nil)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	s.SetOnChanged(func() {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	})

	s.notifyChanged()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("notifyChanged did not invoke the callback")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- s.ServiceShutdown() }()

	select {
	case <-shutdownDone:
		t.Fatal("ServiceShutdown returned before the tracked notifyChanged goroutine finished")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServiceShutdown did not return after the goroutine finished")
	}
}

// TestNotifyChangedRefusesWhileClosing covers the other half of invariant
// 19: once ServiceShutdown has flipped s.closing, later calls must not start
// new work behind its back.
func TestNotifyChangedRefusesWhileClosing(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, nil)

	called := make(chan struct{}, 1)
	s.SetOnChanged(func() { called <- struct{}{} })

	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	s.notifyChanged()

	select {
	case <-called:
		t.Fatal("notifyChanged invoked the callback while the service is closing")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestContextRefusesWithoutStartedService covers invariant 20: context()
// must report ok=false instead of substituting context.Background() when
// ServiceStartup has not run yet.
func TestContextRefusesWithoutStartedService(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	svc, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatal(err)
	}

	if ctx, ok := svc.context(); ok || ctx != nil {
		t.Fatalf("context() = (%v, %v), want (nil, false) before ServiceStartup", ctx, ok)
	}
}

// TestRefreshSourceRefusesWithoutStartedService confirms the context()
// refusal reaches a real caller as an ordinary error instead of silently
// running the fetch under context.Background().
func TestRefreshSourceRefusesWithoutStartedService(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	svc, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatal(err)
	}
	svc.sources = append(svc.sources, &Source{ID: "src-1", Name: "Feed", Type: TypeURL, URL: "https://example.test/feed.json", Enabled: true})

	if _, err := svc.RefreshSource("src-1"); err == nil {
		t.Fatal("expected RefreshSource to refuse before ServiceStartup")
	}
	if _, err := svc.TestSource("https://example.test/feed.json"); err == nil {
		t.Fatal("expected TestSource to refuse before ServiceStartup")
	}
}

// TestScheduleAndRefreshDueNoOpWithoutStartedService confirms the background
// scheduler never runs a refresh under a substitute context: without
// ServiceStartup, refreshDue must not fetch a due source at all instead of
// fetching it under context.Background(), which nothing would ever cancel.
func TestScheduleAndRefreshDueNoOpWithoutStartedService(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	svc, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatal(err)
	}
	fs := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	svc.client = &http.Client{Timeout: feed.FetchTimeout}
	svc.sources = append(svc.sources, &Source{ID: "src-1", Name: "Feed", Type: TypeURL, URL: fs.url(), Enabled: true})

	svc.refreshDue()
	if got := fs.count(); got != 0 {
		t.Fatalf("refreshDue fetched the feed %d times before ServiceStartup, want 0", got)
	}

	svc.wg.Add(1)
	svc.schedule()
	if got := fs.count(); got != 0 {
		t.Fatalf("schedule fetched the feed %d times before ServiceStartup, want 0", got)
	}
}
