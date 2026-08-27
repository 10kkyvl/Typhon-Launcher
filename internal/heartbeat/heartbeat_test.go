package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/clientid"
	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type capturedRequest struct {
	path    string
	body    []byte
	headers http.Header
}

func newCapturingServer(t *testing.T, status int) (*httptest.Server, chan capturedRequest) {
	t.Helper()
	ch := make(chan capturedRequest, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		ch <- capturedRequest{path: r.URL.Path, body: body, headers: r.Header.Clone()}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func newTestService(t *testing.T, srv *httptest.Server, resolve func(string) string) *Service {
	t.Helper()
	if resolve == nil {
		resolve = func(string) string { return "" }
	}
	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	svc, err := NewService(id, resolve)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl
	return svc
}

func waitFor(t *testing.T, ch chan capturedRequest, pathSuffix string) capturedRequest {
	t.Helper()
	for {
		select {
		case req := <-ch:
			if strings.HasSuffix(req.path, pathSuffix) {
				return req
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for request to %s", pathSuffix)
		}
	}
}

func drain(ch chan capturedRequest) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestNewServiceRejectsEmptyIdentity(t *testing.T) {
	if _, err := NewService(clientid.Identity{}, func(string) string { return "" }); err == nil {
		t.Fatal("expected error for empty identity")
	}
}

func TestNewServiceRejectsNilResolver(t *testing.T) {
	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	if _, err := NewService(id, nil); err == nil {
		t.Fatal("expected error for nil resolveGameID")
	}
}

func TestHeartbeatIdleByDefault(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	}()

	req := waitFor(t, reqs, "/presence/heartbeat")
	var p heartbeatPayload
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.State != stateIdle || p.GameID != nil {
		t.Fatalf("expected idle/null before any session, got %+v", p)
	}
	if p.AppVersion == "" || p.OS == "" || p.Arch == "" {
		t.Fatalf("expected app/os/arch to be populated, got %+v", p)
	}
}

func TestHeartbeatNoAuthorizationOrIdentityLeak(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	}()

	req := waitFor(t, reqs, "/presence/heartbeat")
	if auth := req.headers.Get("Authorization"); auth != "" {
		t.Fatalf("unexpected Authorization header: %q", auth)
	}
	body := strings.ToLower(string(req.body))
	for _, forbidden := range []string{"bearer", "token", "email", "username", "@"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("payload leaked identity-like content %q: %s", forbidden, req.body)
		}
	}
}

func TestHeartbeatSessionLifecycle(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, func(catalogID string) string { return catalogID })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	}()

	waitFor(t, reqs, "/presence/heartbeat")

	svc.SessionStarted(library.Game{ID: "local-1", CanonicalGameID: "42"})
	req := waitFor(t, reqs, "/presence/heartbeat")
	var p heartbeatPayload
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.State != statePlaying || p.GameID == nil || *p.GameID != "42" {
		t.Fatalf("expected playing/42, got %+v", p)
	}

	svc.SessionStarted(library.Game{ID: "local-2", CanonicalGameID: "43"})
	req = waitFor(t, reqs, "/presence/heartbeat")
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.GameID == nil || *p.GameID != "43" {
		t.Fatalf("expected latest game 43, got %+v", p)
	}

	svc.SessionStopped("local-2")
	req = waitFor(t, reqs, "/presence/heartbeat")
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.State != statePlaying || p.GameID == nil || *p.GameID != "42" {
		t.Fatalf("expected still playing 42 after stopping the newer session, got %+v", p)
	}

	svc.SessionStopped("local-1")
	req = waitFor(t, reqs, "/presence/heartbeat")
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.State != stateIdle || p.GameID != nil {
		t.Fatalf("expected idle/null after stopping all sessions, got %+v", p)
	}
}

func TestHeartbeatGameIDResolution(t *testing.T) {
	tests := []struct {
		name     string
		resolved string
		wantNil  bool
		want     string
	}{
		{name: "unresolved_by_resolver", resolved: "", wantNil: true},
		{name: "valid_numeric_id", resolved: "12345", want: "12345"},
		{name: "garbage_from_resolver_is_filtered", resolved: "abc123", wantNil: true},
		{name: "too_long_id_is_filtered", resolved: strings.Repeat("9", 21), wantNil: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := newCapturingServer(t, http.StatusNoContent)
			svc := newTestService(t, srv, func(string) string { return tc.resolved })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatalf("ServiceStartup: %v", err)
			}
			defer func() {
				if err := svc.ServiceShutdown(); err != nil {
					t.Fatalf("ServiceShutdown: %v", err)
				}
			}()

			waitFor(t, reqs, "/presence/heartbeat")

			svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "catalog-id"})
			req := waitFor(t, reqs, "/presence/heartbeat")
			var p heartbeatPayload
			if err := json.Unmarshal(req.body, &p); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if tc.wantNil {
				if p.GameID != nil {
					t.Fatalf("expected null game_id, got %q", *p.GameID)
				}
				return
			}
			if p.GameID == nil || *p.GameID != tc.want {
				t.Fatalf("expected game_id %q, got %+v", tc.want, p.GameID)
			}
		})
	}
}

func TestHeartbeatServerErrorDoesNotStopService(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusInternalServerError)
	svc := newTestService(t, srv, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	waitFor(t, reqs, "/presence/heartbeat")

	svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "42"})
	waitFor(t, reqs, "/presence/heartbeat")

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
}

func TestHeartbeatUnreachableServerDoesNotStopService(t *testing.T) {
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := closedSrv.URL
	closedSrv.Close()

	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	svc, err := NewService(id, func(string) string { return "" })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cl, err := newClient(url)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "42"})
	svc.SessionStopped("g1")
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
}

func TestHeartbeatDisconnectOnShutdown(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	waitFor(t, reqs, "/presence/heartbeat")
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	req := waitFor(t, reqs, "/presence/disconnect")
	var p disconnectPayload
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode disconnect payload: %v", err)
	}
	if p.SessionID != svc.identity.SessionID || p.InstallationID != svc.identity.InstallationID {
		t.Fatalf("unexpected disconnect payload: %+v", p)
	}
}

func TestServiceShutdownStopsLoop(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	waitFor(t, reqs, "/presence/heartbeat")

	done := make(chan struct{})
	go func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceShutdown did not return: loop goroutine did not exit")
	}
}

func TestHeartbeatConcurrentSessionUpdates(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, func(id string) string { return id })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	waitFor(t, reqs, "/presence/heartbeat")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("game-%d", n)
			svc.SessionStarted(library.Game{ID: id, CanonicalGameID: fmt.Sprintf("%d", n+1)})
			svc.SessionStopped(id)
		}(i)
	}
	wg.Wait()

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	drain(reqs)
}
