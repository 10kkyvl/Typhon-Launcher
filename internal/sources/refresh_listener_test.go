package sources

import (
	"context"
	"testing"
	"time"
)

func TestRefreshListenerRunsAfterSettleWithoutChanges(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	got := make(chan context.Context, 4)
	s.AddRefreshListener(func(ctx context.Context) { got <- ctx })

	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if summary.Added != 0 || summary.Updated != 0 || summary.Restored != 0 {
		t.Fatalf("summary = %+v, want no changes", summary)
	}
	select {
	case ctx := <-got:
		if ctx != s.ctx {
			t.Fatal("listener must receive the service context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh listener was not called after a refresh without changes")
	}
}

func TestRefreshListenerSkippedWhenRefreshFails(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	called := make(chan struct{}, 1)
	s.AddRefreshListener(func(context.Context) { called <- struct{}{} })
	server.fail(500)

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("refresh must fail")
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case <-called:
		t.Fatal("listener ran after a failed refresh")
	default:
	}
}

func TestRefreshListenerNilIsIgnored(t *testing.T) {
	s, _, _ := testService(t)
	s.AddRefreshListener(nil)
	if len(s.refreshListeners) != 0 {
		t.Fatal("nil listener must not be stored")
	}
}
