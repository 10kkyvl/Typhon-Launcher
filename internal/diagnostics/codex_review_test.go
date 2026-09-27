package diagnostics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodexReviewNoNewRequestAfterOptOut(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	svc := newTestService(t, srv, nil)
	svc.Capture("download", "first", errors.New("first"), false)
	if err := savePending(svc.pendingDir, time.Now(), svc.queue); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { svc.flush(context.Background()); close(done) }()
	<-entered
	svc.SetEnabled(false)
	close(release)
	<-done
	if n := calls.Load(); n != 1 {
		t.Fatalf("started %d requests; expected only the request already in flight before opt-out", n)
	}
}
