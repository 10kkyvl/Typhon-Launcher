package usagestats

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

	"github.com/wailsapp/wails/v3/pkg/application"
)

type capturedRequest struct {
	path string
	body []byte
}

func newCapturingServer(t *testing.T, status int) (*httptest.Server, chan capturedRequest) {
	t.Helper()
	ch := make(chan capturedRequest, 256)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		ch <- capturedRequest{path: r.URL.Path, body: body}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
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

func newTestServiceCustom(t *testing.T, srv *httptest.Server, enabled func() bool, resolve func(string) string) *Service {
	t.Helper()
	if enabled == nil {
		enabled = func() bool { return true }
	}
	if resolve == nil {
		resolve = func(id string) string { return id }
	}
	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	svc, err := NewService(id, enabled, resolve)
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

func TestNewServiceRejectsNilCallbacks(t *testing.T) {
	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	if _, err := NewService(id, nil, func(string) string { return "" }); err == nil {
		t.Fatal("expected error for nil enabled callback")
	}
	if _, err := NewService(id, func() bool { return true }, nil); err == nil {
		t.Fatal("expected error for nil resolveGameID callback")
	}
	if _, err := NewService(clientid.Identity{}, func() bool { return true }, func(string) string { return "" }); err == nil {
		t.Fatal("expected error for empty identity")
	}
}

func TestRecordNoopWhenDisabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, func() bool { return false }, nil)

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "123"}})

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected nothing queued while disabled, got %d", qlen)
	}
	select {
	case req := <-reqs:
		t.Fatalf("expected no request while disabled, got %s", req.body)
	default:
	}
}

func TestRecordSendsBatchWhenEnabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "123"}})
	svc.flush(context.Background())

	req := waitFor(t, reqs, "/telemetry/events")
	var decoded batchPayload
	if err := json.Unmarshal(req.body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.InstallationID != svc.identity.InstallationID || decoded.SessionID != svc.identity.SessionID {
		t.Fatalf("unexpected batch identity: %+v", decoded)
	}
	if len(decoded.Events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d: %+v", len(decoded.Events), decoded.Events)
	}
	if decoded.Events[0].Type != TypeGameStarted || decoded.Events[0].Properties.GameID != "123" {
		t.Fatalf("unexpected event: %+v", decoded.Events[0])
	}
}

func TestBatchPayloadHasNoLeakedIdentifiers(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)

	now := time.Now()
	events := []Event{
		{Type: TypeLauncherSessionStarted, Timestamp: now},
		{Type: TypeLauncherSessionStopped, Timestamp: now, Properties: Properties{DurationSeconds: 10}},
		{Type: TypeGameStarted, Timestamp: now, Properties: Properties{GameID: "1"}},
		{Type: TypeGameStopped, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 5}},
		{Type: TypeDownloadStarted, Timestamp: now, Properties: Properties{GameID: "1"}},
		{Type: TypeDownloadCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2, AverageSpeedBytes: 3}},
		{Type: TypeDownloadFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2, ErrorCode: "network"}},
		{Type: TypeDownloadCancelled, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, BytesTotal: 2}},
		{Type: TypeInstallStarted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "msi_installer"}},
		{Type: TypeInstallCompleted, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "portable", DurationSeconds: 4}},
		{Type: TypeInstallFailed, Timestamp: now, Properties: Properties{GameID: "1", InstallerType: "unknown", DurationSeconds: 4, ErrorCode: "timeout"}},
		{Type: TypeUpdateStarted, Timestamp: now, Properties: Properties{GameID: "1"}},
		{Type: TypeUpdateCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}},
		{Type: TypeUpdateFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "disk_full"}},
		{Type: TypeVerifyStarted, Timestamp: now, Properties: Properties{GameID: "1"}},
		{Type: TypeVerifyCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}},
		{Type: TypeVerifyFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "not_found"}},
		{Type: TypeRepairStarted, Timestamp: now, Properties: Properties{GameID: "1"}},
		{Type: TypeRepairCompleted, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1}},
		{Type: TypeRepairFailed, Timestamp: now, Properties: Properties{GameID: "1", DurationSeconds: 1, ErrorCode: "unknown"}},
	}
	for _, ev := range events {
		svc.Record(ev)
	}
	svc.flush(context.Background())

	req := waitFor(t, reqs, "/telemetry/events")
	body := strings.ToLower(string(req.body))
	forbidden := []string{
		"magnet:", "infohash", "http://", "https://", `\\`, "c:",
		"tracker", ".exe", ".zip", ".7z", ".rar", "@", "username", "email",
	}
	for _, f := range forbidden {
		if strings.Contains(body, strings.ToLower(f)) {
			t.Fatalf("payload leaked forbidden substring %q: %s", f, req.body)
		}
	}
}

func TestSetEnabledFalseClearsQueueAndBlocksSend(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1"}})
	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen == 0 {
		t.Fatal("expected event queued before opting out")
	}

	svc.SetEnabled(false)

	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected queue cleared after opt-out, got %d", qlen)
	}

	svc.flush(context.Background())
	select {
	case req := <-reqs:
		t.Fatalf("expected no request after opt-out, got %s", req.body)
	default:
	}

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "2"}})
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatal("expected Record to stay a no-op while SetEnabled(false) is in effect")
	}

	svc.SetEnabled(true)
	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "3"}})
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 1 {
		t.Fatalf("expected Record to resume after SetEnabled(true), queue has %d", qlen)
	}
}

func TestQueueOverflowDropsOldestNotNewest(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)
	svc.maxQueue = 3

	for i := 1; i <= 5; i++ {
		svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: fmt.Sprintf("%d", i)}})
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.queue) != 3 {
		t.Fatalf("expected queue capped at 3, got %d", len(svc.queue))
	}
	want := []string{"3", "4", "5"}
	for i, w := range want {
		if svc.queue[i].Properties.GameID != w {
			t.Fatalf("expected newest events retained in order %v, got %+v", want, svc.queue)
		}
	}
}

func TestRecordDropsInvalidEvents(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)

	svc.Record(Event{Type: "not_a_real_type", Timestamp: time.Now()})
	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "abc"}})
	svc.Record(Event{Type: TypeDownloadFailed, Timestamp: time.Now(), Properties: Properties{GameID: "1", ErrorCode: "bad code"}})
	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1", DurationSeconds: 5}})

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected all invalid events dropped, queue has %d", qlen)
	}

	svc.flush(context.Background())
	select {
	case req := <-reqs:
		t.Fatalf("expected no request for invalid events, got %s", req.body)
	default:
	}
}

func TestRecordFiltersGameIDResolverGarbage(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, func(string) string { return "not-numeric" })

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "local-catalog-id"}})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.queue) != 0 {
		t.Fatalf("expected event with unresolved game id to be dropped, queue has %d", len(svc.queue))
	}
}

func TestFlushHappensAtThresholdWithoutWaitingForTicker(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)
	svc.flushThreshold = 3
	svc.flushInterval = time.Hour

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

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1"}})
	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "2"}})

	req := waitFor(t, reqs, "/telemetry/events")
	var decoded batchPayload
	if err := json.Unmarshal(req.body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Events) != 3 {
		t.Fatalf("expected batch of exactly 3 events at threshold (session start + 2 recorded), got %d: %+v", len(decoded.Events), decoded.Events)
	}
}

func TestFlushFailureDoesNotBreakSubsequentFlush(t *testing.T) {
	var mu sync.Mutex
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		shouldFail := fail
		mu.Unlock()
		if shouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc := newTestServiceCustom(t, srv, nil, nil)

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1"}})
	svc.flush(context.Background())

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected failed batch to be dropped rather than requeued, got %d", qlen)
	}

	mu.Lock()
	fail = false
	mu.Unlock()

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "2"}})
	svc.flush(context.Background())

	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected queue drained after a successful flush, got %d", qlen)
	}
}

func TestFlushUnreachableServerDoesNotBreakService(t *testing.T) {
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := closedSrv.URL
	closedSrv.Close()

	id := clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
	svc, err := NewService(id, func() bool { return true }, func(s string) string { return s })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cl, err := newClient(url)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl

	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1"}})
	svc.flush(context.Background())

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected dropped batch after unreachable server, got %d", qlen)
	}
}

func TestServiceShutdownFlushesFinalBatch(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)
	svc.flushThreshold = 1000
	svc.flushInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: "1"}})

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	req := waitFor(t, reqs, "/telemetry/events")
	var decoded batchPayload
	if err := json.Unmarshal(req.body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	foundGameStarted := false
	foundStopped := false
	for _, e := range decoded.Events {
		if e.Type == TypeGameStarted {
			foundGameStarted = true
		}
		if e.Type == TypeLauncherSessionStopped {
			foundStopped = true
		}
	}
	if !foundGameStarted || !foundStopped {
		t.Fatalf("expected final flush to include queued and session-stopped events, got %+v", decoded.Events)
	}
}

func TestUsagestatsConcurrentRecordAndSetEnabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestServiceCustom(t, srv, nil, nil)
	svc.flushThreshold = 4
	svc.flushInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.Record(Event{Type: TypeGameStarted, Timestamp: time.Now(), Properties: Properties{GameID: fmt.Sprintf("%d", n+1)}})
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.SetEnabled(n%2 == 0)
		}(i)
	}
	wg.Wait()
	svc.SetEnabled(true)

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	drain(reqs)
}
