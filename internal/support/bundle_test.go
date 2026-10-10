package support

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	refusedLimit       = `{"error":{"code":"ticket_limit"}}`
	refusedBundle      = `{"error":{"code":"ticket_bundle_unavailable","field":"logBundleId"}}`
	refusedBundleLimit = `{"error":{"code":"ticket_bundle_limit","field":"logBundleId"}}`
	refusedNotFound    = `{"error":{"code":"ticket_not_found"}}`
	acceptedBody       = `{"ticket":` + ticketJSON + `,"message":` + messageJSON + `}`
)

type uploads struct{ n atomic.Int32 }

func (u *uploads) next() (string, error) {
	return fmt.Sprintf("BUNDLE%02d", u.n.Add(1)), nil
}

func attachingInput() CreateInput {
	input := validInput()
	input.AttachLogs = true
	return input
}

func sentBundle(t *testing.T, r *rig, index int) string {
	t.Helper()
	calls := r.api.requests()
	if index >= len(calls) {
		t.Fatalf("only %d requests, want request %d", len(calls), index)
	}
	value, has := decodeBody(t, calls[index])["logBundleId"]
	if !has {
		return ""
	}
	return as[string](t, value)
}

func TestRetryAfterARefusedCreateKeepsTheUploadedBundle(t *testing.T) {
	up := &uploads{}
	r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit})
	r.start(t)

	if _, err := r.svc.Create(attachingInput()); apiCode(t, err).Code != CodeTicketLimit {
		t.Fatalf("first Create error = %v", err)
	}
	r.resp.set(http.StatusCreated, acceptedBody)
	if _, err := r.svc.Create(attachingInput()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads = %d, want the first archive reused", got)
	}
	if first, retry := sentBundle(t, r, 0), sentBundle(t, r, 1); first != "BUNDLE01" || retry != "BUNDLE01" {
		t.Fatalf("bundle ids = %q then %q, want BUNDLE01 both times", first, retry)
	}

	if _, err := r.svc.Create(attachingInput()); err != nil {
		t.Fatalf("next request: %v", err)
	}
	if got := up.n.Load(); got != 2 {
		t.Fatalf("uploads = %d, want a new archive once the first one was attached", got)
	}
	if sentBundle(t, r, 2) != "BUNDLE02" {
		t.Fatalf("third request carries %q", sentBundle(t, r, 2))
	}
}

func TestRetryAfterANetworkFailureKeepsTheUploadedBundle(t *testing.T) {
	up := &uploads{}
	r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusBadGateway, body: "<html>bad gateway</html>"})
	r.start(t)

	if _, err := r.svc.Create(attachingInput()); err == nil {
		t.Fatal("Create succeeded through a bad gateway")
	}
	r.resp.set(http.StatusCreated, acceptedBody)
	if _, err := r.svc.Create(attachingInput()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads = %d, want 1", got)
	}
}

func TestBundleTheServerRefusedIsNotReused(t *testing.T) {
	for _, body := range []string{refusedBundle, refusedBundleLimit} {
		t.Run(body, func(t *testing.T) {
			up := &uploads{}
			r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusUnprocessableEntity, body: body})
			r.start(t)

			if _, err := r.svc.Create(attachingInput()); err == nil {
				t.Fatal("Create succeeded")
			}
			r.resp.set(http.StatusCreated, acceptedBody)
			if _, err := r.svc.Create(attachingInput()); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if got := up.n.Load(); got != 2 {
				t.Fatalf("uploads = %d, want a fresh archive after the server refused the first", got)
			}
			if sentBundle(t, r, 1) != "BUNDLE02" {
				t.Fatalf("retry carries %q", sentBundle(t, r, 1))
			}
		})
	}
}

func TestBundleOfAMissingTicketIsNotReused(t *testing.T) {
	up := &uploads{}
	r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusNotFound, body: refusedNotFound})
	r.start(t)

	if _, err := r.svc.Reply(1042, "more", true); err == nil {
		t.Fatal("Reply succeeded")
	}
	r.resp.set(http.StatusCreated, acceptedBody)
	if _, err := r.svc.Reply(1042, "more", true); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := up.n.Load(); got != 2 {
		t.Fatalf("uploads = %d, want 2", got)
	}
}

func TestBundleIsKeptPerTarget(t *testing.T) {
	up := &uploads{}
	r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit})
	r.start(t)

	if _, err := r.svc.Reply(1042, "more", true); err == nil {
		t.Fatal("Reply succeeded")
	}
	if _, err := r.svc.Reply(1042, "more", true); err == nil {
		t.Fatal("Reply succeeded")
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads after two replies to one request = %d, want 1", got)
	}

	if _, err := r.svc.Reply(2000, "more", true); err == nil {
		t.Fatal("Reply succeeded")
	}
	if _, err := r.svc.Create(attachingInput()); err == nil {
		t.Fatal("Create succeeded")
	}
	if got := up.n.Load(); got != 3 {
		t.Fatalf("uploads = %d, want one archive for each of the three targets", got)
	}
}

func TestBundleIsNotReusedAcrossAccountsOrAfterItGrewOld(t *testing.T) {
	t.Run("another session token", func(t *testing.T) {
		up := &uploads{}
		token := "first"
		r := newRig(t, rigOptions{
			uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit,
			token: func() (string, error) { return token, nil },
		})
		r.start(t)

		if _, err := r.svc.Create(attachingInput()); err == nil {
			t.Fatal("Create succeeded")
		}
		token = "second"
		if _, err := r.svc.Create(attachingInput()); err == nil {
			t.Fatal("Create succeeded")
		}
		if got := up.n.Load(); got != 2 {
			t.Fatalf("uploads = %d, want the archive of the first session left alone", got)
		}
	})

	t.Run("older than the reuse window", func(t *testing.T) {
		up := &uploads{}
		r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit})
		now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
		r.svc.now = func() time.Time { return now }
		r.start(t)

		if _, err := r.svc.Create(attachingInput()); err == nil {
			t.Fatal("Create succeeded")
		}
		now = now.Add(bundleReuseWindow - time.Minute)
		if _, err := r.svc.Create(attachingInput()); err == nil {
			t.Fatal("Create succeeded")
		}
		if got := up.n.Load(); got != 1 {
			t.Fatalf("uploads inside the window = %d, want 1", got)
		}
		now = now.Add(2 * time.Minute)
		if _, err := r.svc.Create(attachingInput()); err == nil {
			t.Fatal("Create succeeded")
		}
		if got := up.n.Load(); got != 2 {
			t.Fatalf("uploads after the window = %d, want 2", got)
		}
	})
}

func TestRequestWithoutLogsLeavesTheKeptBundleAlone(t *testing.T) {
	up := &uploads{}
	r := newRig(t, rigOptions{uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit})
	r.start(t)

	if _, err := r.svc.Create(attachingInput()); err == nil {
		t.Fatal("Create succeeded")
	}
	if _, err := r.svc.Create(validInput()); err == nil {
		t.Fatal("Create succeeded")
	}
	r.resp.set(http.StatusCreated, acceptedBody)
	if _, err := r.svc.Create(attachingInput()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads = %d, want 1", got)
	}
	if sentBundle(t, r, 1) != "" {
		t.Fatal("a request without logs carried a bundle")
	}
}

func TestShutdownAbandonsARunningUpload(t *testing.T) {
	started := make(chan struct{})
	r := newRig(t, rigOptions{uploadCtx: func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}})
	if err := r.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Create(attachingInput())
		done <- err
	}()
	<-started
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Create error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Create is still waiting for an upload after shutdown")
	}
	if got := len(r.api.requests()); got != 0 {
		t.Fatalf("%d requests went out after the upload was abandoned", got)
	}
}
